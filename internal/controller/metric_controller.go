/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"github.com/openmcp-project/metrics-operator/internal/clientoptl"
	"github.com/openmcp-project/metrics-operator/internal/common"
	"github.com/openmcp-project/metrics-operator/internal/config"
	orc "github.com/openmcp-project/metrics-operator/internal/orchestrator"
)

const (
	// RequeueAfterError is the time to requeue the metric after an error
	RequeueAfterError = 2 * time.Minute
)

// NewMetricReconciler creates a new MetricReconciler
func NewMetricReconciler(mgr ctrl.Manager) *MetricReconciler {
	return &MetricReconciler{
		log: mgr.GetLogger().WithName("controllers").WithName("Metric"),

		inCli:      mgr.GetClient(),
		RestConfig: mgr.GetConfig(),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorder("Metric-controller"),
	}
}

// MetricReconciler reconciles a Metric object
type MetricReconciler struct {
	log logr.Logger

	inCli      client.Client
	Scheme     *runtime.Scheme
	RestConfig *rest.Config
	Recorder   events.EventRecorder
}

// GetClient returns the client
func (r *MetricReconciler) getClient() client.Client {
	return r.inCli
}

// GetRestConfig returns the rest config
func (r *MetricReconciler) getRestConfig() *rest.Config {
	return r.RestConfig
}

// getDataSinkCredentials fetches DataSink configuration and credentials
func (r *MetricReconciler) getDataSinkCredentials(ctx context.Context, metric *v1alpha1.Metric, l logr.Logger) (common.DataSinkCredentials, error) {
	retriever := NewDataSinkCredentialsRetriever(r.getClient(), r.Recorder)
	return retriever.GetDataSinkCredentials(ctx, metric.Spec.DataSinkRef, metric, l)
}

func (r *MetricReconciler) scheduleNextReconciliation(metric *v1alpha1.Metric) ctrl.Result {

	elapsed := time.Since(metric.Status.Observation.Timestamp.Time)
	return ctrl.Result{
		RequeueAfter: metric.Spec.Interval.Duration - elapsed,
	}
}

func (r *MetricReconciler) shouldReconcile(metric *v1alpha1.Metric) bool {
	if metric.Status.Observation.LatestValue == "" || metric.Status.Observation.Timestamp.Time.IsZero() {
		return true
	}
	elapsed := time.Since(metric.Status.Observation.Timestamp.Time)
	return elapsed >= metric.Spec.Interval.Duration
}

func (r *MetricReconciler) handleGetError(err error, log logr.Logger) (ctrl.Result, error) {
	// we'll ignore not-found errors, since they can't be fixed by an immediate
	// requeue (we'll need to wait for a new notification), and we can also get them
	// on delete requests.
	if apierrors.IsNotFound(err) {
		log.Info("Metric not found")
		return ctrl.Result{RequeueAfter: RequeueAfterError}, nil
	}
	log.Error(err, "unable to fetch Metric")
	return ctrl.Result{RequeueAfter: RequeueAfterError}, err
}

// +kubebuilder:rbac:groups=metrics.openmcp.cloud,resources=metrics,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metrics.openmcp.cloud,resources=metrics/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metrics.openmcp.cloud,resources=metrics/finalizers,verbs=update
// +kubebuilder:rbac:groups=metrics.openmcp.cloud,resources=datasinks,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

// Reconcile handles the reconciliation of a Metric object
//
//nolint:gocyclo
func (r *MetricReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := r.log.WithValues("namespace", req.NamespacedName, "name", req.Name)

	l.Info("Reconciling Metric")

	/*
			1. Load the generic metric using the client
		 	All method should take the context to allow for cancellation (like CancellationToken)
	*/
	metric := v1alpha1.Metric{}
	if errLoad := r.getClient().Get(ctx, req.NamespacedName, &metric); errLoad != nil {
		return r.handleGetError(errLoad, l)
	}

	// Defer status update to ensure it's always called
	defer func() {
		if err := r.getClient().Status().Update(ctx, &metric); err != nil {
			l.Error(err, "Failed to update Metric status")
		}
	}()

	// Initialize Ready condition if not present
	if meta.FindStatusCondition(metric.Status.Conditions, v1alpha1.TypeReady) == nil {
		metric.SetConditions(common.ReadyUnknown("Reconciling", "Initial reconciliation"))
	}

	// Check if enough time has passed since the last reconciliation
	if !r.shouldReconcile(&metric) {
		return r.scheduleNextReconciliation(&metric), nil
	}

	/*
		1.1 Get DataSink configuration and credentials
	*/
	credentials, err := r.getDataSinkCredentials(ctx, &metric, l)
	if err != nil {
		metric.SetConditions(common.ReadyFalse("DataSinkUnavailable", err.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		return ctrl.Result{RequeueAfter: RequeueAfterError}, err
	}

	/*
		1.2 Create QueryConfig to query the resources in the K8S cluster or external cluster based on the kubeconfig secret reference
	*/
	queryConfig, err := createQC(ctx, metric.Spec.RemoteClusterAccessRef, r)
	if err != nil {
		metric.SetConditions(common.ReadyFalse("QueryConfigCreationFailed", err.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		return ctrl.Result{RequeueAfter: RequeueAfterError}, err
	}

	metricClient, errCli := clientoptl.NewMetricClient(ctx, credentials.Host, credentials.Token)
	if errCli != nil {
		metric.SetConditions(common.ReadyFalse("OTLPClientCreationFailed", errCli.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		l.Error(errCli, fmt.Sprintf("metric '%s' failed to create OTel client, re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errCli
	}
	defer func() {
		if err := metricClient.Close(ctx); err != nil {
			l.Error(err, "Failed to close metric client during metric reconciliation", "metric", metric.Name)
		}
	}() // Ensure exporter is shut down

	metricClient.SetMeter("metric")

	gaugeMetric, errGauge := metricClient.NewMetric(metric.Name)
	if errGauge != nil {
		metric.SetConditions(common.ReadyFalse("MetricCreationFailed", errGauge.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		l.Error(errGauge, fmt.Sprintf("metric '%s' failed to create OTel gauge, re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errGauge
	}
	/*
		2. Create a new orchestrator
	*/
	orchestrator, errOrch := orc.NewOrchestrator(credentials, queryConfig).WithMetric(metric, gaugeMetric) // Pass gaugeMetric
	if errOrch != nil {
		metric.SetConditions(common.ReadyFalse("OrchestratorCreationFailed", errOrch.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		l.Error(errOrch, "unable to create metric orchestrator monitor")
		r.Recorder.Eventf(&metric, nil, "Warning", "OrchestratorCreation", "ReconcileMetric", "unable to create orchestrator")
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errOrch
	}

	result, errMon := orchestrator.Handler.Monitor(ctx)

	if errMon != nil {
		metric.SetConditions(common.ReadyFalse("MonitoringFailed", errMon.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		l.Error(errMon, fmt.Sprintf("metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errMon
	}

	errExport := metricClient.ExportMetrics(ctx)

	/*
		3. Update the status of the metric with conditions and phase
	*/
	switch result.Phase {
	case v1alpha1.PhaseActive:
		metric.SetConditions(common.Available(result.Message))
		r.Recorder.Eventf(&metric, nil, "Normal", "MetricAvailable", "ReconcileMetric", result.Message)
	case v1alpha1.PhaseFailed:
		l.Error(result.Error, result.Message, "reason", result.Reason)
		metric.SetConditions(common.Error(result.Message))
		r.Recorder.Eventf(&metric, nil, "Warning", "MetricFailed", "ReconcileMetric", result.Message)
	case v1alpha1.PhasePending:
		metric.SetConditions(common.Creating())
		r.Recorder.Eventf(&metric, nil, "Normal", "MetricPending", "ReconcileMetric", result.Message)
	}

	cObs := result.Observation.(*v1alpha1.MetricObservation)

	// Set Ready condition based on export result
	if errExport != nil {
		metric.SetConditions(common.ReadyFalse("MetricExportFailed", errExport.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		l.Error(errExport, fmt.Sprintf("metric '%s' failed to export, re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
	} else {
		metric.SetConditions(common.ReadyTrue("Metric reconciled successfully"))
		metric.Status.Ready = v1alpha1.StatusStringTrue
	}

	metric.Status.Observation = v1alpha1.MetricObservation{
		Timestamp:   result.Observation.GetTimestamp(),
		LatestValue: cObs.LatestValue,
		Dimensions:  cObs.Dimensions,
	}

	// Update LastReconcileTime
	metric.Status.Observation.Timestamp.Time = metav1.Now().Time

	// Note: Status update is handled by the defer function at the beginning

	/*
		4. Requeue the metric after the frequency or after 2 minutes if an error occurred
	*/
	var requeueTime time.Duration
	if result.Error != nil || errExport != nil { // Requeue faster on monitor or export error
		requeueTime = RequeueAfterError
	} else {
		requeueTime = metric.Spec.Interval.Duration
	}

	l.Info(fmt.Sprintf("metric '%s' re-queued for execution in %v\n", metric.Spec.Name, requeueTime))

	return ctrl.Result{
		RequeueAfter: requeueTime,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MetricReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Metric{}).
		Complete(r)
}

func createQC(ctx context.Context, rcaRef *v1alpha1.RemoteClusterAccessRef, r InsightReconciler) (orc.QueryConfig, error) {
	var queryConfig orc.QueryConfig
	// Kubernetes client to the external cluster if defined
	if rcaRef != nil {
		qc, err := config.CreateExternalQueryConfig(ctx, rcaRef, r.getClient())
		if err != nil {
			return orc.QueryConfig{}, err
		}
		queryConfig = *qc
	} else {
		// local cluster name (where operator is deployed)
		clusterName, _ := getClusterInfo(r.getRestConfig())
		queryConfig = orc.QueryConfig{Client: r.getClient(), RestConfig: *r.getRestConfig(), ClusterName: &clusterName}
	}
	return queryConfig, nil
}

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/SAP/metrics-operator/api/v1alpha1"
	"github.com/SAP/metrics-operator/internal/clientoptl"
	"github.com/SAP/metrics-operator/internal/common"
	"github.com/SAP/metrics-operator/internal/config"
	orc "github.com/SAP/metrics-operator/internal/orchestrator"
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
		Recorder:   mgr.GetEventRecorderFor("Metric-controller"),
	}
}

// MetricReconciler reconciles a Metric object
type MetricReconciler struct {
	log logr.Logger

	inCli      client.Client
	Scheme     *runtime.Scheme
	RestConfig *rest.Config
	Recorder   record.EventRecorder
}

// GetClient returns the client
func (r *MetricReconciler) getClient() client.Client {
	return r.inCli
}

// GetRestConfig returns the rest config
func (r *MetricReconciler) getRestConfig() *rest.Config {
	return r.RestConfig
}

func (r *MetricReconciler) scheduleNextReconciliation(metric *v1alpha1.Metric) (ctrl.Result, error) {

	elapsed := time.Since(metric.Status.Observation.Timestamp.Time)
	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: metric.Spec.CheckInterval.Duration - elapsed,
	}, nil
}

func (r *MetricReconciler) shouldReconcile(metric *v1alpha1.Metric) bool {
	if metric.Status.Observation.LatestValue == "" || metric.Status.Observation.Timestamp.Time.IsZero() {
		return true
	}
	elapsed := time.Since(metric.Status.Observation.Timestamp.Time)
	return elapsed >= metric.Spec.CheckInterval.Duration
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

// +kubebuilder:rbac:groups=metrics.cloud.sap,resources=metrics,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metrics.cloud.sap,resources=metrics/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metrics.cloud.sap,resources=metrics/finalizers,verbs=update

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

	// Check if enough time has passed since the last reconciliation
	if !r.shouldReconcile(&metric) {
		return r.scheduleNextReconciliation(&metric)
	}

	/*
		1.1 Get the Secret that holds the Dynatrace credentials
	*/
	secret, errSecret := common.GetCredentialsSecret(ctx, r.getClient())
	if errSecret != nil {
		l.Error(errSecret, fmt.Sprintf("unable to fetch secret '%s' in namespace '%s' that stores the credentials to data sink", common.SecretName, common.SecretNameSpace))
		r.Recorder.Event(&metric, "Error", "SecretNotFound", fmt.Sprintf("unable to fetch secret '%s' in namespace '%s' that stores the credentials to data sink", common.SecretName, common.SecretNameSpace))
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errSecret
	}

	credentials := common.GetCredentialData(secret)

	/*
		1.2 Create QueryConfig to query the resources in the K8S cluster or external cluster based on the kubeconfig secret reference
	*/
	queryConfig, err := createQC(ctx, &metric.Spec.RemoteClusterAccessRef, r)
	if err != nil {
		return ctrl.Result{RequeueAfter: RequeueAfterError}, err
	}

	metricClient, errCli := clientoptl.NewMetricClient(ctx, credentials.Host, credentials.Path, credentials.Token)
	if errCli != nil {
		l.Error(errCli, fmt.Sprintf("metric '%s' failed to create OTel client, re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		// TODO: Update status?
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
		l.Error(errGauge, fmt.Sprintf("metric '%s' failed to create OTel gauge, re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		// TODO: Update status?
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errGauge
	}
	/*
		2. Create a new orchestrator
	*/
	orchestrator, errOrch := orc.NewOrchestrator(credentials, queryConfig).WithMetric(metric, gaugeMetric) // Pass gaugeMetric
	if errOrch != nil {
		l.Error(errOrch, "unable to create metric orchestrator monitor")
		r.Recorder.Event(&metric, "Warning", "OrchestratorCreation", "unable to create orchestrator")
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errOrch
	}

	result, errMon := orchestrator.Handler.Monitor(ctx)

	if errMon != nil {
		metric.Status.Ready = v1alpha1.StatusFalse
		l.Error(errMon, fmt.Sprintf("metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		// Update status before returning
		_ = r.getClient().Status().Update(ctx, &metric) // Best effort status update on error
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errMon
	}

	errExport := metricClient.ExportMetrics(ctx)
	if errExport != nil {
		metric.Status.Ready = v1alpha1.StatusFalse
		l.Error(errExport, fmt.Sprintf("metric '%s' failed to export, re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
	} else {
		metric.Status.Ready = v1alpha1.StatusTrue
	}

	/*
		3. Update the status of the metric with conditions and phase
	*/
	switch result.Phase {
	case v1alpha1.PhaseActive:
		metric.SetConditions(common.Available(result.Message))
		r.Recorder.Event(&metric, "Normal", "MetricAvailable", result.Message)
	case v1alpha1.PhaseFailed:
		l.Error(result.Error, result.Message, "reason", result.Reason)
		metric.SetConditions(common.Error(result.Message))
		r.Recorder.Event(&metric, "Warning", "MetricFailed", result.Message)
	case v1alpha1.PhasePending:
		metric.SetConditions(common.Creating())
		r.Recorder.Event(&metric, "Normal", "MetricPending", result.Message)
	}

	cObs := result.Observation.(*v1alpha1.MetricObservation)

	// Override Ready status if export failed
	if errExport != nil {
		metric.Status.Ready = "False"
	}
	// metric.Status.Observation = v1beta1.MetricObservation{Timestamp: result.Observation.GetTimestamp(), Dimensions: cObs.Dimensions, LatestValue: cObs.LatestValue}
	metric.Status.Observation = v1alpha1.MetricObservation{
		Timestamp:   result.Observation.GetTimestamp(),
		LatestValue: cObs.LatestValue,
		Dimensions:  cObs.Dimensions,
	}

	// Update LastReconcileTime
	metric.Status.Observation.Timestamp.Time = metav1.Now().Time

	// conditions are not persisted until the status is updated
	errUp := r.getClient().Status().Update(ctx, &metric)
	if errUp != nil {
		l.Error(errUp, fmt.Sprintf("metric '%s' failed to update status, re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errUp
	}

	/*
		4. Requeue the metric after the frequency or after 2 minutes if an error occurred
	*/
	var requeueTime time.Duration
	if result.Error != nil || errExport != nil { // Requeue faster on monitor or export error
		requeueTime = RequeueAfterError
	} else {
		requeueTime = metric.Spec.CheckInterval.Duration
	}

	l.Info(fmt.Sprintf("metric '%s' re-queued for execution in %v\n", metric.Spec.Name, requeueTime))

	return ctrl.Result{
		Requeue:      true,
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

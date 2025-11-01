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
	"net/url"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"

	"github.com/openmcp-project/metrics-operator/internal/clientoptl"
	"github.com/openmcp-project/metrics-operator/internal/common"
	"github.com/openmcp-project/metrics-operator/internal/config"
	"github.com/openmcp-project/metrics-operator/internal/orchestrator"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
)

// NewManagedMetricReconciler creates a new ManagedMetricReconciler
func NewManagedMetricReconciler(mgr ctrl.Manager) *ManagedMetricReconciler {
	return &ManagedMetricReconciler{
		inClient:     mgr.GetClient(),
		inRestConfig: mgr.GetConfig(),
		Scheme:       mgr.GetScheme(),
		Recorder:     mgr.GetEventRecorderFor("managedmetrics-controller"),
	}
}

func (r *ManagedMetricReconciler) getClient() client.Client {
	return r.inClient
}

func (r *ManagedMetricReconciler) getRestConfig() *rest.Config {
	return r.inRestConfig
}

func (r *ManagedMetricReconciler) scheduleNextReconciliation(metric *v1alpha1.ManagedMetric) ctrl.Result {
	elapsed := time.Since(metric.Status.Observation.Timestamp.Time)
	return ctrl.Result{
		RequeueAfter: metric.Spec.Interval.Duration - elapsed,
	}
}

func (r *ManagedMetricReconciler) shouldReconcile(metric *v1alpha1.ManagedMetric) bool {
	if metric.Status.Observation.Timestamp.Time.IsZero() {
		return true
	}
	elapsed := time.Since(metric.Status.Observation.Timestamp.Time)
	return elapsed >= metric.Spec.Interval.Duration
}

// ManagedMetricReconciler reconciles a ManagedMetric object
type ManagedMetricReconciler struct {
	inClient     client.Client
	inRestConfig *rest.Config
	Scheme       *runtime.Scheme

	Recorder record.EventRecorder
}

// getDataSinkCredentials fetches DataSink configuration and credentials
func (r *ManagedMetricReconciler) getDataSinkCredentials(ctx context.Context, managedMetric *v1alpha1.ManagedMetric, l logr.Logger) (common.DataSinkCredentials, error) {
	retriever := NewDataSinkCredentialsRetriever(r.getClient(), r.Recorder)
	return retriever.GetDataSinkCredentials(ctx, managedMetric.Spec.DataSinkRef, managedMetric, l)
}

// +kubebuilder:rbac:groups=metrics.openmcp.cloud,resources=managedmetrics,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metrics.openmcp.cloud,resources=managedmetrics/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metrics.openmcp.cloud,resources=managedmetrics/finalizers,verbs=update
// +kubebuilder:rbac:groups=metrics.openmcp.cloud,resources=datasinks,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
//
//nolint:gocyclo
func (r *ManagedMetricReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var l = log.FromContext(ctx)

	/*
			1. Load the managed metric using the client
		 	All method should take the context to allow for cancellation (like CancellationToken)
	*/
	metric := v1alpha1.ManagedMetric{}
	if errLoad := r.inClient.Get(ctx, req.NamespacedName, &metric); errLoad != nil {
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can also get them
		// on delete requests.
		if apierrors.IsNotFound(errLoad) {
			l.Info("Managed Metric not found")
			return ctrl.Result{RequeueAfter: RequeueAfterError}, nil
		}
		l.Error(errLoad, "unable to fetch Managed Metric")
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errLoad
	}

	// Defer status update to ensure it's always called
	defer func() {
		if err := r.inClient.Status().Update(ctx, &metric); err != nil {
			l.Error(err, "Failed to update ManagedMetric status")
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
	queryConfig, err := createQueryConfig(ctx, metric.Spec.RemoteClusterAccessRef, r)
	if err != nil {
		metric.SetConditions(common.ReadyFalse("QueryConfigCreationFailed", err.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		return ctrl.Result{RequeueAfter: RequeueAfterError}, err
	}

	/*
		1.3 Create OTel metric client and gauge metric
	*/
	metricClient, errCli := clientoptl.NewMetricClient(ctx, credentials.Host, credentials.Token)
	if errCli != nil {
		metric.SetConditions(common.ReadyFalse("OTLPClientCreationFailed", errCli.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		l.Error(errCli, fmt.Sprintf("managed metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errCli
	}

	defer func() {
		if err := metricClient.Close(ctx); err != nil {
			l.Error(err, "Failed to close metric client during managed metric reconciliation", "metric", metric.Name)
		}
	}()

	// Set meter name for managed metrics
	metricClient.SetMeter("managed")

	gaugeMetric, errGauge := metricClient.NewMetric(metric.Name)
	if errGauge != nil {
		metric.SetConditions(common.ReadyFalse("MetricCreationFailed", errGauge.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		l.Error(errGauge, fmt.Sprintf("managed metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errGauge
	}

	/*
		2. Create a new orchestrator
	*/
	orchestrator, errOrch := orchestrator.NewOrchestrator(credentials, queryConfig).WithManaged(metric, gaugeMetric)
	if errOrch != nil {
		metric.SetConditions(common.ReadyFalse("OrchestratorCreationFailed", errOrch.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		l.Error(errOrch, "unable to create managed metric orchestrator monitor")
		r.Recorder.Event(&metric, "Warning", "OrchestratorCreation", "unable to create orchestrator")
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errOrch
	}

	result, errMon := orchestrator.Handler.Monitor(ctx)

	if errMon != nil {
		metric.SetConditions(common.ReadyFalse("MonitoringFailed", errMon.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		l.Error(errMon, fmt.Sprintf("managed metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errMon
	}

	/*
		2.1 Export metrics to data sink
	*/
	errExport := metricClient.ExportMetrics(ctx)

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

	// Set Ready condition based on export result
	if errExport != nil {
		metric.SetConditions(common.ReadyFalse("MetricExportFailed", errExport.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		l.Error(errExport, fmt.Sprintf("managed metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
	} else {
		metric.SetConditions(common.ReadyTrue("Managed metric reconciled successfully"))
		metric.Status.Ready = v1alpha1.StatusStringTrue
	}

	// Update the observation timestamp to track when this reconciliation happened
	metric.Status.Observation = v1alpha1.ManagedObservation{
		Timestamp: metav1.Now(),
		Resources: result.Observation.GetValue(),
	}

	// Note: Status update is handled by the defer function at the beginning

	/*
		4. Requeue the metric after the frequency or after 2 minutes if an error occurred
	*/
	var requeueTime time.Duration
	if result.Error != nil || errExport != nil {
		requeueTime = RequeueAfterError
	} else {
		requeueTime = metric.Spec.Interval.Duration
	}

	l.Info(fmt.Sprintf("managed metric '%s' re-queued for execution in %v\n", metric.Spec.Name, requeueTime))

	return ctrl.Result{
		RequeueAfter: requeueTime,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedMetricReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ManagedMetric{}).
		Complete(r)
}

func createQueryConfig(ctx context.Context, rcaRef *v1alpha1.RemoteClusterAccessRef, r InsightReconciler) (orchestrator.QueryConfig, error) {
	var queryConfig orchestrator.QueryConfig
	// Kubernetes client to the external cluster if defined
	if rcaRef != nil {
		qc, err := config.CreateExternalQueryConfig(ctx, rcaRef, r.getClient())
		if err != nil {
			return orchestrator.QueryConfig{}, err
		}
		queryConfig = *qc
	} else {
		// local cluster name (where operator is deployed)
		clusterName, _ := getClusterInfo(r.getRestConfig())
		queryConfig = orchestrator.QueryConfig{Client: r.getClient(), RestConfig: *r.getRestConfig(), ClusterName: &clusterName}
	}
	return queryConfig, nil
}

func getClusterInfo(config *rest.Config) (string, error) {
	if config.Host == "" {
		return "", fmt.Errorf("config.Host is empty")
	}

	// Parse the host URL
	u, err := url.Parse(config.Host)
	if err != nil {
		return "", fmt.Errorf("failed to parse host URL: %w", err)
	}

	// Extract the hostname
	hostname := u.Hostname()

	// debugging only
	if hostname == "127.0.0.1" {
		return "localhost", nil
	}

	// Remove any prefix (like "kubernetes" or "kubernetes.default.svc")
	parts := strings.Split(hostname, ".")
	clusterName := parts[0]

	return clusterName, nil

}

// OrchestratorFactory is a function type for creating orchestrators
type OrchestratorFactory func(creds common.DataSinkCredentials, qConfig orchestrator.QueryConfig) *orchestrator.Orchestrator

// QueryConfigFactory is a function type for creating query configs
type QueryConfigFactory func(ctx context.Context, rcaRef *v1alpha1.RemoteClusterAccessRef, r InsightReconciler) (orchestrator.QueryConfig, error)

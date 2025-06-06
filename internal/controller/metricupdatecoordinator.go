package controller

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/SAP/metrics-operator/api/v1alpha1"
	"github.com/SAP/metrics-operator/internal/clientoptl"
	"github.com/SAP/metrics-operator/internal/common"
	"github.com/SAP/metrics-operator/internal/config"
	orc "github.com/SAP/metrics-operator/internal/orchestrator"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MetricUpdateCoordinator is responsible for orchestrating the update of a single metric
// when triggered by an event or a scheduled reconciliation.
type MetricUpdateCoordinator struct {
	client.Client
	Log        logr.Logger
	RestConfig *rest.Config
	Recorder   record.EventRecorder
	Scheme     *runtime.Scheme // Correct type for Kubernetes scheme

	// Concurrency control fields
	lastProcessed  map[string]time.Time
	debounceWindow time.Duration
}

// NewMetricUpdateCoordinator creates a new MetricUpdateCoordinator.
func NewMetricUpdateCoordinator(k8sClient client.Client, logger logr.Logger, config *rest.Config, recorder record.EventRecorder, scheme *runtime.Scheme) *MetricUpdateCoordinator {
	return &MetricUpdateCoordinator{
		Client:         k8sClient,
		Log:            logger.WithName("MetricUpdateCoordinator"),
		RestConfig:     config,
		Recorder:       recorder,
		Scheme:         scheme,
		lastProcessed:  make(map[string]time.Time),
		debounceWindow: 500 * time.Millisecond, // Reduced debounce window for better responsiveness
	}
}

// RequestMetricUpdate is called by the ResourceEventHandler (or potentially a polling mechanism)
// to process a metric. The eventObj and eventGVK are for context, might not be directly used
// if the metric's own spec is the sole driver for fetching data.
func (muc *MetricUpdateCoordinator) RequestMetricUpdate(metricNamespacedName string, eventGVK schema.GroupVersionKind, _ interface{}) {
	ctx := context.Background() // Consider passing a more specific context
	log := muc.Log.WithValues("metric", metricNamespacedName, "triggeringGVK", eventGVK.String())
	log.Info("MetricUpdateCoordinator: Metric update requested")

	// Check for duplicate requests and apply debouncing
	// TEMPORARILY DISABLED FOR DEBUGGING DELETE EVENTS
	// if muc.shouldSkipDuplicateRequest(metricNamespacedName, log) {
	//	return
	// }

	// The metricNamespacedName should be "namespace/name"
	namespace, name, err := cache.SplitMetaNamespaceKey(metricNamespacedName)
	if err != nil {
		log.Error(err, "Failed to split metric namespaced name", "metricNamespacedName", metricNamespacedName)
		return
	}

	var metric v1alpha1.Metric
	if err := muc.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &metric); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Metric not found, perhaps deleted.")
			// TODO: Ensure TargetRegistry is cleaned up if a metric is deleted.
			// This might be handled by the main Metric controller watching Metric CRs.
			return
		}
		log.Error(err, "Failed to get Metric CR for update")
		return
	}

	// TODO: Add a check here: if this metric is *not* event-driven (e.g. has a flag),
	// then maybe it shouldn't be processed by event-triggered updates.
	// Or, the event-driven path always re-evaluates. For now, assume all are processable.

	log.Info("Processing metric update", "metricName", metric.Spec.Name)
	if err := muc.processMetric(ctx, &metric, log); err != nil {
		log.Error(err, "Error processing metric")
		// Status update with error is handled within processMetric
	}
}

// processMetric contains the core logic to fetch, calculate, and export a single metric.
// This is refactored from the original MetricReconciler.Reconcile.
func (muc *MetricUpdateCoordinator) processMetric(ctx context.Context, metric *v1alpha1.Metric, log logr.Logger) error {
	// Setup phase
	credentials, err := muc.setupCredentials(ctx, metric, log)
	if err != nil {
		return err
	}

	queryConfig, err := muc.setupQueryConfig(ctx, metric, log)
	if err != nil {
		return err
	}

	metricClient, err := muc.setupMetricClient(ctx, credentials, metric, log)
	if err != nil {
		return err
	}
	defer muc.closeMetricClient(ctx, metricClient, log, metric.Name)

	gaugeMetric, err := muc.createGaugeMetric(metricClient, metric, log)
	if err != nil {
		return err
	}

	orchestrator, err := muc.createOrchestrator(credentials, queryConfig, metric, gaugeMetric, log)
	if err != nil {
		return err
	}

	// Execution phase
	result, errMon := orchestrator.Handler.Monitor(ctx)
	errExport := muc.exportMetrics(ctx, metricClient, log)

	// Status update phase
	finalError := muc.updateMetricStatus(ctx, metric, result, errMon, errExport, log)
	return finalError
}

// setupCredentials handles credential retrieval and error handling
func (muc *MetricUpdateCoordinator) setupCredentials(ctx context.Context, metric *v1alpha1.Metric, log logr.Logger) (common.DataSinkCredentials, error) {
	secret, err := common.GetCredentialsSecret(ctx, muc.Client)
	if err != nil {
		log.Error(err, "Unable to fetch credentials secret", "secretName", common.SecretName, "secretNamespace", common.SecretNameSpace)
		muc.Recorder.Event(metric, "Error", "SecretNotFound", fmt.Sprintf("unable to fetch secret '%s' in namespace '%s'", common.SecretName, common.SecretNameSpace))
		metric.Status.Ready = v1alpha1.StatusFalse
		metric.SetConditions(common.Error(fmt.Sprintf("Credentials secret %s/%s not found: %s", common.SecretNameSpace, common.SecretName, err.Error())))
		if updateErr := muc.updateMetricStatusWithRetry(ctx, metric, 3, log); updateErr != nil {
			log.Error(updateErr, "Failed to update metric status after secret error")
		}
		return common.DataSinkCredentials{}, err
	}
	return common.GetCredentialData(secret), nil
}

// setupQueryConfig creates the query configuration
func (muc *MetricUpdateCoordinator) setupQueryConfig(ctx context.Context, metric *v1alpha1.Metric, log logr.Logger) (orc.QueryConfig, error) {
	queryConfig, err := muc.createCoordinatorQueryConfig(ctx, metric.Spec.RemoteClusterAccessRef)
	if err != nil {
		log.Error(err, "Failed to create query config")
		metric.Status.Ready = v1alpha1.StatusFalse
		metric.SetConditions(common.Error("Failed to create query config: " + err.Error()))
		_ = muc.updateMetricStatusWithRetry(ctx, metric, 3, log)
		return orc.QueryConfig{}, err
	}
	return queryConfig, nil
}

// setupMetricClient creates the OTel metric client
func (muc *MetricUpdateCoordinator) setupMetricClient(ctx context.Context, credentials common.DataSinkCredentials, metric *v1alpha1.Metric, log logr.Logger) (*clientoptl.MetricClient, error) {
	metricClient, err := clientoptl.NewMetricClient(ctx, credentials.Host, credentials.Path, credentials.Token)
	if err != nil {
		log.Error(err, "Failed to create OTel client")
		metric.Status.Ready = v1alpha1.StatusFalse
		metric.SetConditions(common.Error("Failed to create OTel client: " + err.Error()))
		_ = muc.updateMetricStatusWithRetry(ctx, metric, 3, log)
		return nil, err
	}
	metricClient.SetMeter("metric")
	return metricClient, nil
}

// closeMetricClient safely closes the metric client
func (muc *MetricUpdateCoordinator) closeMetricClient(ctx context.Context, metricClient *clientoptl.MetricClient, log logr.Logger, metricName string) {
	if err := metricClient.Close(ctx); err != nil {
		log.Error(err, "Failed to close metric client", "metric", metricName)
	}
}

// createGaugeMetric creates the gauge metric
func (muc *MetricUpdateCoordinator) createGaugeMetric(metricClient *clientoptl.MetricClient, metric *v1alpha1.Metric, log logr.Logger) (*clientoptl.Metric, error) {
	gaugeMetric, err := metricClient.NewMetric(metric.Name)
	if err != nil {
		log.Error(err, "Failed to create OTel gauge")
		metric.Status.Ready = v1alpha1.StatusFalse
		metric.SetConditions(common.Error("Failed to create OTel gauge: " + err.Error()))
		_ = muc.updateMetricStatusWithRetry(context.Background(), metric, 3, log)
		return nil, err
	}
	return gaugeMetric, nil
}

// createOrchestrator creates the metric orchestrator
func (muc *MetricUpdateCoordinator) createOrchestrator(credentials common.DataSinkCredentials, queryConfig orc.QueryConfig, metric *v1alpha1.Metric, gaugeMetric *clientoptl.Metric, log logr.Logger) (*orc.Orchestrator, error) {
	orchestrator, err := orc.NewOrchestrator(credentials, queryConfig).WithMetric(*metric, gaugeMetric)
	if err != nil {
		log.Error(err, "Unable to create metric orchestrator")
		muc.Recorder.Event(metric, "Warning", "OrchestratorCreation", "unable to create orchestrator")
		metric.Status.Ready = v1alpha1.StatusFalse
		metric.SetConditions(common.Error("Failed to create orchestrator: " + err.Error()))
		_ = muc.updateMetricStatusWithRetry(context.Background(), metric, 3, log)
		return nil, err
	}
	return orchestrator, nil
}

// exportMetrics handles metric export
func (muc *MetricUpdateCoordinator) exportMetrics(ctx context.Context, metricClient *clientoptl.MetricClient, log logr.Logger) error {
	err := metricClient.ExportMetrics(ctx)
	if err != nil {
		log.Error(err, "Failed to export metrics")
	}
	return err
}

// updateMetricStatus handles the complex status update logic
func (muc *MetricUpdateCoordinator) updateMetricStatus(ctx context.Context, metric *v1alpha1.Metric, result orc.MonitorResult, errMon, errExport error, log logr.Logger) error {
	finalError := errMon

	// Handle monitoring errors
	if errMon != nil {
		log.Error(errMon, "Orchestrator monitoring failed")
		metric.Status.Ready = v1alpha1.StatusFalse
		metric.SetConditions(common.Error("Monitoring failed: " + errMon.Error()))
	}

	// Handle export errors
	if errExport != nil {
		log.Error(errExport, "Failed to export metrics")
		metric.Status.Ready = v1alpha1.StatusFalse
		if finalError == nil {
			finalError = errExport
		}
		if errMon == nil {
			metric.SetConditions(common.Error("Metric export failed: " + errExport.Error()))
		}
	} else if errMon == nil {
		metric.Status.Ready = v1alpha1.StatusTrue
	}

	// Process monitor results
	if errMon == nil {
		muc.processMonitorResult(metric, result)
	} else {
		muc.handleMonitorFailure(metric, finalError)
	}

	// Update observation timestamp
	metric.Status.Observation.Timestamp = metav1.Now()

	// Update status with retry
	if errUp := muc.updateMetricStatusWithRetry(ctx, metric, 5, log); errUp != nil {
		log.Error(errUp, "Failed to update metric status after retries")
		if finalError == nil {
			finalError = errUp
		}
	}

	return finalError
}

// processMonitorResult handles successful monitor results
func (muc *MetricUpdateCoordinator) processMonitorResult(metric *v1alpha1.Metric, result orc.MonitorResult) {
	switch result.Phase {
	case v1alpha1.PhaseActive:
		muc.handleActivePhase(metric, result)
	case v1alpha1.PhaseFailed:
		muc.handleFailedPhase(metric, result)
	case v1alpha1.PhasePending:
		muc.handlePendingPhase(metric, result)
	}

	muc.updateObservation(metric, result)
}

// handleActivePhase processes active phase results
func (muc *MetricUpdateCoordinator) handleActivePhase(metric *v1alpha1.Metric, result orc.MonitorResult) {
	if metric.Status.Ready == v1alpha1.StatusTrue {
		metric.SetConditions(common.Available(result.Message))
		muc.Recorder.Event(metric, "Normal", "MetricAvailable", result.Message)
	} else {
		// Monitor active, but export status unclear and Ready is false
		metric.SetConditions(common.Error("Metric available (monitor) but overall status not ready and no export error"))
	}
}

// handleFailedPhase processes failed phase results
func (muc *MetricUpdateCoordinator) handleFailedPhase(metric *v1alpha1.Metric, result orc.MonitorResult) {
	muc.Log.Error(result.Error, "Metric processing resulted in failed phase (from monitor result)", "reason", result.Reason, "message", result.Message)
	metric.SetConditions(common.Error(result.Message))
	muc.Recorder.Event(metric, "Warning", "MetricFailed", result.Message)
}

// handlePendingPhase processes pending phase results
func (muc *MetricUpdateCoordinator) handlePendingPhase(metric *v1alpha1.Metric, result orc.MonitorResult) {
	metric.SetConditions(common.Creating())
	muc.Recorder.Event(metric, "Normal", "MetricPending", result.Message)
}

// updateObservation updates the metric observation from monitor results
func (muc *MetricUpdateCoordinator) updateObservation(metric *v1alpha1.Metric, result orc.MonitorResult) {
	if cObs, ok := result.Observation.(*v1alpha1.MetricObservation); ok {
		metric.Status.Observation = v1alpha1.MetricObservation{
			Timestamp:   result.Observation.GetTimestamp(),
			LatestValue: cObs.LatestValue,
			Dimensions:  cObs.Dimensions,
		}
	} else if result.Observation != nil {
		muc.Log.Error(fmt.Errorf("unexpected observation type: %T", result.Observation), "Failed to cast observation from monitor result")
	}
}

// handleMonitorFailure handles cases where monitoring itself failed
func (muc *MetricUpdateCoordinator) handleMonitorFailure(metric *v1alpha1.Metric, finalError error) {
	if metric.Status.Ready == "" {
		metric.Status.Ready = v1alpha1.StatusFalse
	}
	if len(metric.Status.Conditions) == 0 && finalError != nil {
		metric.SetConditions(common.Error(finalError.Error()))
	} else if len(metric.Status.Conditions) == 0 {
		metric.SetConditions(common.Error("Metric processing failed due to monitoring error"))
	}
}

// updateMetricStatusWithRetry implements retry logic with exponential backoff for status updates
func (muc *MetricUpdateCoordinator) updateMetricStatusWithRetry(ctx context.Context, metric *v1alpha1.Metric, maxRetries int, log logr.Logger) error {
	for attempt := 0; attempt < maxRetries; attempt++ {
		// For the first attempt, use the metric as-is
		// For subsequent attempts, fetch fresh copy to get latest resource version
		targetMetric := metric
		if attempt > 0 {
			var freshMetric v1alpha1.Metric
			if err := muc.Get(ctx, client.ObjectKeyFromObject(metric), &freshMetric); err != nil {
				log.Error(err, "Failed to get fresh metric for retry", "attempt", attempt+1)
				return err
			}

			// Copy the status updates to the fresh metric
			freshMetric.Status = metric.Status
			targetMetric = &freshMetric
		}

		// Attempt the status update
		if err := muc.Status().Update(ctx, targetMetric); err != nil {
			if apierrors.IsConflict(err) && attempt < maxRetries-1 {
				// Calculate exponential backoff: 100ms, 200ms, 400ms, 800ms, 1600ms
				backoffMs := int64(100 * math.Pow(2, float64(attempt)))
				backoff := time.Duration(backoffMs) * time.Millisecond

				log.V(1).Info("Status update conflict, retrying with backoff",
					"attempt", attempt+1,
					"maxRetries", maxRetries,
					"backoff", backoff,
					"error", err.Error())

				time.Sleep(backoff)
				continue
			}
			// Non-conflict error or max retries reached
			return fmt.Errorf("failed to update metric status after %d attempts: %w", attempt+1, err)
		}

		// Success
		if attempt > 0 {
			log.Info("Status update succeeded after retry", "attempt", attempt+1)
		}
		return nil
	}

	return fmt.Errorf("failed to update status after %d retries", maxRetries)
}

// createCoordinatorQueryConfig is a helper to create QueryConfig, similar to createQC in metric_controller.
// It uses the MetricUpdateCoordinator's client and rest.Config.
func (muc *MetricUpdateCoordinator) createCoordinatorQueryConfig(ctx context.Context, rcaRef *v1alpha1.RemoteClusterAccessRef) (orc.QueryConfig, error) {
	var queryConfig orc.QueryConfig
	if rcaRef != nil {
		qc, err := config.CreateExternalQueryConfig(ctx, rcaRef, muc.Client)
		if err != nil {
			return orc.QueryConfig{}, err
		}
		queryConfig = *qc
	} else {
		// For local cluster, we need client and rest.Config.
		// getClusterInfo was originally from reconciler, might need to adapt or pass scheme.
		// Let's assume getClusterInfo can be called with just rest.Config for now.
		// Or, if clusterName is not strictly needed by orchestrator for local, simplify.
		// The original getClusterInfo is not exported.
		// For simplicity, let's set a default/placeholder name or make it optional in QueryConfig.
		// Alternatively, the orchestrator might not need clusterName if client is local.

		// Placeholder for cluster name for local execution
		localClusterName := "local-cluster" // Or fetch from a config map or env var if needed

		queryConfig = orc.QueryConfig{Client: muc.Client, RestConfig: *muc.RestConfig, ClusterName: &localClusterName}
	}
	return queryConfig, nil
}

// Helper to get client (satisfies parts of InsightReconciler interface implicitly for createQC logic)
func (muc *MetricUpdateCoordinator) getClient() client.Client {
	return muc.Client
}

// Helper to get rest config (satisfies parts of InsightReconciler interface implicitly for createQC logic)
func (muc *MetricUpdateCoordinator) getRestConfig() *rest.Config {
	return muc.RestConfig
}

// Ensure MetricUpdateCoordinator implements MetricUpdateCoordinatorInterface
var _ MetricUpdateCoordinatorInterface = &MetricUpdateCoordinator{}

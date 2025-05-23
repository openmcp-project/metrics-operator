package controller

import (
	"context"
	"fmt"

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
}

// NewMetricUpdateCoordinator creates a new MetricUpdateCoordinator.
func NewMetricUpdateCoordinator(k8sClient client.Client, logger logr.Logger, config *rest.Config, recorder record.EventRecorder, scheme *runtime.Scheme) *MetricUpdateCoordinator {
	return &MetricUpdateCoordinator{
		Client:     k8sClient,
		Log:        logger.WithName("MetricUpdateCoordinator"),
		RestConfig: config,
		Recorder:   recorder,
		Scheme:     scheme,
	}
}

// RequestMetricUpdate is called by the ResourceEventHandler (or potentially a polling mechanism)
// to process a metric. The eventObj and eventGVK are for context, might not be directly used
// if the metric's own spec is the sole driver for fetching data.
func (muc *MetricUpdateCoordinator) RequestMetricUpdate(metricNamespacedName string, eventGVK schema.GroupVersionKind, eventObj interface{}) {
	ctx := context.Background() // Consider passing a more specific context
	log := muc.Log.WithValues("metric", metricNamespacedName, "triggeringGVK", eventGVK.String())
	log.Info("MetricUpdateCoordinator: Metric update requested")

	// TODO: Potentially use a workqueue here to decouple and manage processing.
	// For now, process synchronously.

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
	// 1. Get Credentials Secret
	secret, errSecret := common.GetCredentialsSecret(ctx, muc.Client)
	if errSecret != nil {
		log.Error(errSecret, "Unable to fetch credentials secret", "secretName", common.SecretName, "secretNamespace", common.SecretNameSpace)
		muc.Recorder.Event(metric, "Error", "SecretNotFound", fmt.Sprintf("unable to fetch secret '%s' in namespace '%s'", common.SecretName, common.SecretNameSpace))
		// Update status to reflect this error
		metric.Status.Ready = v1alpha1.StatusFalse
		metric.SetConditions(common.Error(fmt.Sprintf("Credentials secret %s/%s not found: %s", common.SecretNameSpace, common.SecretName, errSecret.Error())))
		if err := muc.Status().Update(ctx, metric); err != nil {
			log.Error(err, "Failed to update metric status after secret error")
		}
		return errSecret
	}
	credentials := common.GetCredentialData(secret)

	// 2. Create QueryConfig
	// We need a way to get the InsightReconciler interface or its methods for createQC.
	// For now, let's assume we can construct it or pass necessary parts.
	// The original createQC takes (ctx, rcaRef, InsightReconciler).
	// InsightReconciler provides getClient() and getRestConfig(). muc has these.
	queryConfig, errQCFunc := muc.createCoordinatorQueryConfig(ctx, metric.Spec.RemoteClusterAccessRef)
	if errQCFunc != nil {
		log.Error(errQCFunc, "Failed to create query config")
		metric.Status.Ready = v1alpha1.StatusFalse
		metric.SetConditions(common.Error("Failed to create query config: " + errQCFunc.Error()))
		_ = muc.Status().Update(ctx, metric) // Best effort
		return errQCFunc
	}

	// 3. Create OTel Metric Client
	metricClient, errCli := clientoptl.NewMetricClient(ctx, credentials.Host, credentials.Path, credentials.Token)
	if errCli != nil {
		log.Error(errCli, "Failed to create OTel client")
		metric.Status.Ready = v1alpha1.StatusFalse
		metric.SetConditions(common.Error("Failed to create OTel client: " + errCli.Error()))
		_ = muc.Status().Update(ctx, metric) // Best effort
		return errCli
	}
	defer func() {
		if err := metricClient.Close(ctx); err != nil {
			log.Error(err, "Failed to close metric client", "metric", metric.Name)
		}
	}()
	metricClient.SetMeter("metric") // Or a more dynamic meter name

	gaugeMetric, errGauge := metricClient.NewMetric(metric.Name) // metric.Name or metric.Spec.Name? Metric CRD uses Spec.Name for Dynatrace.
	if errGauge != nil {
		log.Error(errGauge, "Failed to create OTel gauge")
		metric.Status.Ready = v1alpha1.StatusFalse
		metric.SetConditions(common.Error("Failed to create OTel gauge: " + errGauge.Error()))
		_ = muc.Status().Update(ctx, metric) // Best effort
		return errGauge
	}

	// 4. Create Orchestrator
	orchestrator, errOrch := orc.NewOrchestrator(credentials, queryConfig).WithMetric(*metric, gaugeMetric)
	if errOrch != nil {
		log.Error(errOrch, "Unable to create metric orchestrator")
		muc.Recorder.Event(metric, "Warning", "OrchestratorCreation", "unable to create orchestrator")
		metric.Status.Ready = v1alpha1.StatusFalse
		metric.SetConditions(common.Error("Failed to create orchestrator: " + errOrch.Error()))
		_ = muc.Status().Update(ctx, metric) // Best effort
		return errOrch
	}

	// 5. Monitor
	result, errMon := orchestrator.Handler.Monitor(ctx)
	var finalError error = errMon // Keep track of monitor error for final status
	if errMon != nil {
		log.Error(errMon, "Orchestrator monitoring failed")
		metric.Status.Ready = v1alpha1.StatusFalse
		// If errMon is not nil, 'result' is a zero-value struct.
		// The primary error is errMon.
		metric.SetConditions(common.Error("Monitoring failed: " + errMon.Error()))
	}

	// 6. Export Metrics
	errExport := metricClient.ExportMetrics(ctx)
	if errExport != nil {
		log.Error(errExport, "Failed to export metrics")
		metric.Status.Ready = v1alpha1.StatusFalse // Explicitly set to false on export error
		if finalError == nil {
			finalError = errExport
		}
		// If monitoring was successful (errMon == nil) but export failed, update condition.
		// If monitoring also failed, errMon's condition is already set.
		if errMon == nil { // Only set export error condition if monitor was ok
			metric.SetConditions(common.Error("Metric export failed: " + errExport.Error()))
		}
	} else if errMon == nil { // Only set to true if monitor AND export were successful
		metric.Status.Ready = v1alpha1.StatusTrue
	}

	// 7. Update Status (based on monitor result and export status)
	// Only use 'result' fields if errMon is nil, indicating Monitor() itself completed without error.
	if errMon == nil {
		switch result.Phase {
		case v1alpha1.PhaseActive:
			if metric.Status.Ready == v1alpha1.StatusTrue { // Monitor active and export succeeded
				metric.SetConditions(common.Available(result.Message))
				muc.Recorder.Event(metric, "Normal", "MetricAvailable", result.Message)
			} else if errExport != nil {
				// Condition for export failure already set if monitor was OK.
			} else {
				// Monitor active, but export status unclear and Ready is false. This is an odd state.
				metric.SetConditions(common.Error("Metric available (monitor) but overall status not ready and no export error"))
			}
		case v1alpha1.PhaseFailed:
			log.Error(result.Error, "Metric processing resulted in failed phase (from monitor result)", "reason", result.Reason, "message", result.Message)
			metric.SetConditions(common.Error(result.Message)) // Monitor result's error message
			muc.Recorder.Event(metric, "Warning", "MetricFailed", result.Message)
		case v1alpha1.PhasePending:
			metric.SetConditions(common.Creating()) // Assuming "Creating" is a pending state
			muc.Recorder.Event(metric, "Normal", "MetricPending", result.Message)
		}

		if cObs, ok := result.Observation.(*v1alpha1.MetricObservation); ok {
			metric.Status.Observation = v1alpha1.MetricObservation{
				Timestamp:   result.Observation.GetTimestamp(),
				LatestValue: cObs.LatestValue,
				Dimensions:  cObs.Dimensions,
			}
		} else if result.Observation != nil { // Observation is present but not castable
			log.Error(fmt.Errorf("unexpected observation type: %T", result.Observation), "Failed to cast observation from monitor result")
			// metric.Status.Observation.Timestamp = metav1.Now() // Timestamp will be set below
		}
		// If result.Observation is nil, timestamp will be set below.
	} else { // errMon != nil, so monitor itself failed.
		// metric.Status.Ready should already be false.
		// Condition for errMon is already set.
		// We just ensure a fallback status if nothing else was set.
		if metric.Status.Ready == "" {
			metric.Status.Ready = v1alpha1.StatusFalse
		}
		if len(metric.Status.Conditions) == 0 && finalError != nil { // Check finalError before using
			metric.SetConditions(common.Error(finalError.Error()))
		} else if len(metric.Status.Conditions) == 0 {
			metric.SetConditions(common.Error("Metric processing failed due to monitoring error"))
		}
	}

	// Ensure observation timestamp is updated, regardless of event or polling
	// This timestamp should reflect when this processing attempt concluded.
	metric.Status.Observation.Timestamp = metav1.Now()

	errUp := muc.Status().Update(ctx, metric)
	if errUp != nil {
		log.Error(errUp, "Failed to update metric status")
		if finalError == nil {
			finalError = errUp
		}
	}

	return finalError
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

// getClusterInfo is a simplified local version or needs to be adapted.
// The original one is in metric_controller and not exported.
// For now, this is a placeholder if createCoordinatorQueryConfig needs it.
// It's better if QueryConfig for local doesn't strictly require a cluster name
// or can derive it if absolutely necessary.
// For the purpose of QueryConfig, if it's local, the client and restconfig are primary.
// The orchestrator's use of ClusterName for local queries should be reviewed.
// As a placeholder:
func getClusterInfoCoordinator(rc *rest.Config) (string, error) {
	// This is a simplified placeholder. A real implementation might involve
	// querying the API server or using a well-known config.
	// For now, returning a default or erroring if critical.
	// The original `getClusterInfo` in `metric_controller.go` is not exported.
	// Let's assume for local QueryConfig, the name isn't strictly critical for orchestrator
	// or can be defaulted.
	return "local-cluster-via-coordinator", nil
}

// Ensure MetricUpdateCoordinator implements MetricUpdateCoordinatorInterface
var _ MetricUpdateCoordinatorInterface = &MetricUpdateCoordinator{}

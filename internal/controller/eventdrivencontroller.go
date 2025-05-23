package controller

import (
	"context"
	"time"

	"github.com/SAP/metrics-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EventDrivenController manages the event-driven metric collection system.
// It watches Metric CRs and coordinates the dynamic informer setup.
type EventDrivenController struct {
	client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	RestConfig *rest.Config
	Recorder   record.EventRecorder

	// Core components
	targetRegistry          *TargetRegistry
	dynamicInformerManager  *DynamicInformerManager
	resourceEventHandler    *ResourceEventHandler
	metricUpdateCoordinator *MetricUpdateCoordinator

	// Dynamic client for creating informers
	dynamicClient dynamic.Interface
}

// NewEventDrivenController creates a new EventDrivenController.
func NewEventDrivenController(mgr ctrl.Manager) *EventDrivenController {
	// Create dynamic client
	dynClient, err := dynamic.NewForConfig(mgr.GetConfig())
	if err != nil {
		// This is a critical error during setup
		panic("Failed to create dynamic client: " + err.Error())
	}

	// Initialize core components
	targetRegistry := NewTargetRegistry()

	metricUpdateCoordinator := NewMetricUpdateCoordinator(
		mgr.GetClient(),
		mgr.GetLogger().WithName("MetricUpdateCoordinator"),
		mgr.GetConfig(),
		mgr.GetEventRecorderFor("EventDriven-controller"),
		mgr.GetScheme(),
	)

	resourceEventHandler := NewResourceEventHandler(
		mgr.GetLogger().WithName("ResourceEventHandler"),
		targetRegistry,
		metricUpdateCoordinator,
	)

	dynamicInformerManager := NewDynamicInformerManager(
		dynClient,
		10*time.Minute, // Default resync period
		mgr.GetLogger().WithName("DynamicInformerManager"),
		resourceEventHandler,
	)

	return &EventDrivenController{
		Client:                  mgr.GetClient(),
		Log:                     mgr.GetLogger().WithName("EventDrivenController"),
		Scheme:                  mgr.GetScheme(),
		RestConfig:              mgr.GetConfig(),
		Recorder:                mgr.GetEventRecorderFor("EventDriven-controller"),
		targetRegistry:          targetRegistry,
		dynamicInformerManager:  dynamicInformerManager,
		resourceEventHandler:    resourceEventHandler,
		metricUpdateCoordinator: metricUpdateCoordinator,
		dynamicClient:           dynClient,
	}
}

// Reconcile handles changes to Metric CRs and updates the dynamic informer setup.
func (edc *EventDrivenController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := edc.Log.WithValues("metric", req.NamespacedName)
	log.Info("Reconciling Metric for event-driven setup")

	// Fetch the Metric CR
	var metric v1alpha1.Metric
	if err := edc.Get(ctx, req.NamespacedName, &metric); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Metric was deleted, unregister it
			log.Info("Metric deleted, unregistering from target registry")
			edc.targetRegistry.Unregister(req.NamespacedName)

			// Update dynamic informers based on new target set
			uniqueTargets := edc.targetRegistry.GetUniqueTargets()
			edc.dynamicInformerManager.EnsureInformers(ctx, uniqueTargets)

			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get Metric")
		return ctrl.Result{}, err
	}

	// TODO: Add a check here for whether this metric should use event-driven updates
	// For now, assume all metrics are event-driven capable

	// Register or update the metric's target interest
	if err := edc.targetRegistry.Register(&metric); err != nil {
		log.Error(err, "Failed to register metric in target registry")
		edc.Recorder.Event(&metric, "Warning", "RegistrationFailed", "Failed to register metric for event-driven updates")
		return ctrl.Result{RequeueAfter: 2 * time.Minute}, err
	}

	log.Info("Metric registered in target registry", "targetGVK", metric.Spec.Target)

	// Update dynamic informers based on the new target set
	uniqueTargets := edc.targetRegistry.GetUniqueTargets()
	edc.dynamicInformerManager.EnsureInformers(ctx, uniqueTargets)

	log.Info("Dynamic informers updated", "uniqueTargetsCount", len(uniqueTargets))

	// Trigger initial metric collection for this metric
	// This ensures the metric gets an observation even if no resource events occur immediately
	log.Info("Triggering initial metric collection", "metric", req.NamespacedName)
	edc.metricUpdateCoordinator.RequestMetricUpdate(
		req.NamespacedName.String(),
		metric.Spec.Target.GVK(),
		nil, // No specific triggering object for initial collection
	)

	// Record successful registration
	edc.Recorder.Event(&metric, "Normal", "EventDrivenEnabled", "Metric registered for event-driven updates")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (edc *EventDrivenController) SetupWithManager(mgr ctrl.Manager) error {
	// Use the simpler controller builder API
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Metric{}).
		Complete(edc)
}

// Start initializes the event-driven system.
// This should be called after the manager starts to ensure all informers are ready.
func (edc *EventDrivenController) Start(ctx context.Context) error {
	edc.Log.Info("Starting EventDrivenController")

	// Start the dynamic informer manager
	edc.dynamicInformerManager.Start(ctx)

	// Wait for dynamic informer caches to sync
	if !edc.dynamicInformerManager.WaitForCacheSync(ctx) {
		edc.Log.Error(nil, "Failed to sync dynamic informer caches")
		return nil // Don't return error to avoid crashing the manager
	}

	edc.Log.Info("EventDrivenController started successfully")
	return nil
}

// Stop gracefully shuts down the event-driven system.
func (edc *EventDrivenController) Stop() {
	edc.Log.Info("Stopping EventDrivenController")
	edc.dynamicInformerManager.Stop()
}

// GetTargetRegistry returns the target registry for testing or external access.
func (edc *EventDrivenController) GetTargetRegistry() *TargetRegistry {
	return edc.targetRegistry
}

// GetMetricUpdateCoordinator returns the metric update coordinator for testing or external access.
func (edc *EventDrivenController) GetMetricUpdateCoordinator() *MetricUpdateCoordinator {
	return edc.metricUpdateCoordinator
}

package controller

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
)

// DynamicInformerManager manages dynamic informers for arbitrary Kubernetes resource types.
type DynamicInformerManager struct {
	mu sync.RWMutex

	dynClient          dynamic.Interface
	informerFactory    dynamicinformer.DynamicSharedInformerFactory // Correct type
	activeInformers    map[string]informers.GenericInformer
	activeStoppers     map[string]chan struct{}      // To stop individual informers
	resourceEvtHandler ResourceEventHandlerInterface // Interface for handling events
	gvrDiscovery       *GVRDiscoveryService          // Dynamic GVK to GVR discovery
	log                logr.Logger
}

// ResourceEventHandlerInterface defines the contract for handling resource events.
// This will be implemented by the ResourceEventHandler component.
type ResourceEventHandlerInterface interface {
	OnAdd(obj interface{}, gvk schema.GroupVersionKind)
	OnUpdate(oldObj, newObj interface{}, gvk schema.GroupVersionKind)
	OnDelete(obj interface{}, gvk schema.GroupVersionKind)
}

// targetKey generates a unique string key for a TargetResourceIdentifier
func targetKey(target TargetResourceIdentifier) string {
	return target.GVK.String() + "|" + target.Namespace + "|" + target.Selector.String()
}

// NewDynamicInformerManager creates a new DynamicInformerManager.
func NewDynamicInformerManager(dynClient dynamic.Interface, defaultResync time.Duration, logger logr.Logger, eventHandler ResourceEventHandlerInterface, gvrDiscovery *GVRDiscoveryService) *DynamicInformerManager {
	// We'll create namespace-specific factories as needed, so no global factory here
	return &DynamicInformerManager{
		dynClient:          dynClient,
		informerFactory:    nil, // Will create namespace-specific factories
		activeInformers:    make(map[string]informers.GenericInformer),
		activeStoppers:     make(map[string]chan struct{}),
		resourceEvtHandler: eventHandler,
		gvrDiscovery:       gvrDiscovery,
		log:                logger.WithName("DynamicInformerManager"),
	}
}

// EnsureInformers reconciles the set of active informers based on the desired targets.
// It starts new informers for new targets and stops informers for targets no longer needed.
func (dim *DynamicInformerManager) EnsureInformers(ctx context.Context, targets []TargetResourceIdentifier) {
	dim.mu.Lock()
	defer dim.mu.Unlock()

	// Stop informers for targets that are no longer needed
	for existingTargetKey, stopper := range dim.activeStoppers {
		// Find the corresponding target from the current targets list
		targetStillNeeded := false
		for _, target := range targets {
			if targetKey(target) == existingTargetKey {
				targetStillNeeded = true
				break
			}
		}

		if !targetStillNeeded {
			dim.log.Info("Stopping informer for target", "targetKey", existingTargetKey)
			close(stopper) // Signal the informer's goroutine to stop
			delete(dim.activeInformers, existingTargetKey)
			delete(dim.activeStoppers, existingTargetKey)
		}
	}

	// Start informers for new targets
	for _, target := range targets {
		targetKeyStr := targetKey(target)

		// Check if this target already has an active informer
		if _, found := dim.activeInformers[targetKeyStr]; !found {
			dim.log.Info("Starting informer for target",
				"gvk", target.GVK,
				"namespace", target.Namespace,
				"selector", target.Selector.String(),
				"targetKey", targetKeyStr)

			// Use GVR discovery service to get the correct resource name
			gvr, err := dim.gvrDiscovery.GetGVR(ctx, target.GVK)
			if err != nil {
				dim.log.Error(err, "Failed to discover GVR for target", "gvk", target.GVK)
				// Skip this target and continue with others
				continue
			}

			// Create a namespace-specific factory for this target
			var factory dynamicinformer.DynamicSharedInformerFactory
			if target.Namespace != "" {
				// For namespaced resources, create a namespace-specific factory
				factory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(
					dim.dynClient,
					10*time.Minute, // Default resync period
					target.Namespace,
					nil,
				)
			} else {
				// For cluster-scoped resources, use all-namespace factory
				factory = dynamicinformer.NewFilteredDynamicSharedInformerFactory(
					dim.dynClient,
					10*time.Minute,
					"",
					nil,
				)
			}

			// Get the GenericInformer from the namespace-specific factory
			genericInformer := factory.ForResource(gvr)

			// Get the underlying SharedIndexInformer to add event handlers
			sharedInformer := genericInformer.Informer()

			// Add event handlers
			sharedInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					dim.log.Info("DynamicInformer Event: Add", "gvk", target.GVK.String(), "namespace", target.Namespace)
					if dim.resourceEvtHandler != nil {
						dim.resourceEvtHandler.OnAdd(obj, target.GVK)
					}
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					dim.log.Info("DynamicInformer Event: Update", "gvk", target.GVK.String(), "namespace", target.Namespace)
					if dim.resourceEvtHandler != nil {
						dim.resourceEvtHandler.OnUpdate(oldObj, newObj, target.GVK)
					}
				},
				DeleteFunc: func(obj interface{}) {
					dim.log.Info("DynamicInformer Event: Delete", "gvk", target.GVK.String(), "namespace", target.Namespace)
					if dim.resourceEvtHandler != nil {
						dim.resourceEvtHandler.OnDelete(obj, target.GVK)
					}
				},
			})

			// Start the factory immediately
			factory.Start(ctx.Done())

			stopper := make(chan struct{})
			dim.activeInformers[targetKeyStr] = genericInformer // Store the GenericInformer
			dim.activeStoppers[targetKeyStr] = stopper

			// Start the informer. The factory manages the shared underlying machinery.
			// The factory itself needs to be started, typically once.
			// For dynamic informers, starting the factory might not be enough,
			// individual informers might need a go routine.
			// Let's assume the factory handles this for now.
			// The SharedInformerFactory.Start(stopper) method starts all informers.
			// We will call factory.Start() once, globally.
			// For individual dynamic informers, they should start when event handlers are added
			// and the factory is running.
		}
	}
	// The factory itself should be started once, usually in the main operator setup.
	// dim.informerFactory.Start(ctx.Done()) // This would start all informers in the factory.
	// This needs careful consideration: when and how to start the factory vs individual informers.
	// For now, we assume the factory is started elsewhere, and adding handlers to
	// an informer obtained from it makes it active.
}

// Start initiates the informer factory. This should be called once.
func (dim *DynamicInformerManager) Start(ctx context.Context) {
	dim.log.Info("Starting DynamicInformerManager - namespace-specific factories will be started as needed")
	// With the new approach, factories are started individually when informers are created
	// No global factory to start here
}

// Stop shuts down all active informers.
func (dim *DynamicInformerManager) Stop() {
	dim.mu.Lock()
	defer dim.mu.Unlock()
	dim.log.Info("Stopping all dynamic informers")
	for target, stopper := range dim.activeStoppers {
		close(stopper)
		delete(dim.activeInformers, target)
		delete(dim.activeStoppers, target)
	}
	// Note: The factory itself is stopped by the context passed to Start.
}

// WaitForCacheSync waits for all caches of managed informers to sync.
// Returns true if all caches have synced, false if context is cancelled.
func (dim *DynamicInformerManager) WaitForCacheSync(ctx context.Context) bool {
	syncFuncs := []cache.InformerSynced{}
	dim.mu.RLock()
	for _, informer := range dim.activeInformers {
		syncFuncs = append(syncFuncs, informer.Informer().HasSynced) // Corrected: access HasSynced via Informer()
	}
	dim.mu.RUnlock()

	if len(syncFuncs) == 0 {
		dim.log.V(1).Info("No active informers to sync.")
		return true
	}

	dim.log.Info("Waiting for dynamic informer caches to sync", "count", len(syncFuncs))
	// The factory's WaitForCacheSync waits for all informers started by the factory.
	// However, we are managing informers somewhat individually.
	// Let's use the factory's method if it correctly reflects all *active* informers we care about.
	// Or, we can call cache.WaitForCacheSync directly.

	// The SharedDynamicInformerFactory doesn't have a WaitForCacheSync method.
	// We need to call cache.WaitForCacheSync with the HasSynced funcs of our active informers.
	return cache.WaitForCacheSync(ctx.Done(), syncFuncs...)
}

// GetListerForTarget returns a generic lister for a given target.
// Returns nil if no informer is active for the target.
func (dim *DynamicInformerManager) GetListerForTarget(target TargetResourceIdentifier) cache.GenericLister {
	dim.mu.RLock()
	defer dim.mu.RUnlock()

	targetKeyStr := targetKey(target)
	if informer, found := dim.activeInformers[targetKeyStr]; found {
		return informer.Lister()
	}

	dim.log.V(1).Info("No active informer found for target to get lister", "target", target)
	return nil
}

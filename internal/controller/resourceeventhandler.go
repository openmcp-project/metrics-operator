package controller

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
)

// MetricUpdateCoordinatorInterface defines the contract for triggering metric updates.
// This will be implemented by the MetricUpdateCoordinator component.
type MetricUpdateCoordinatorInterface interface {
	RequestMetricUpdate(metricNamespacedName string, gvk schema.GroupVersionKind, eventObj interface{})
}

// ResourceEventHandler handles events from dynamic informers,
// maps them to interested Metric CRs, and triggers updates.
type ResourceEventHandler struct {
	logger            logr.Logger
	targetRegistry    *TargetRegistry                  // To find interested metrics
	updateCoordinator MetricUpdateCoordinatorInterface // To trigger updates
}

// NewResourceEventHandler creates a new ResourceEventHandler.
func NewResourceEventHandler(logger logr.Logger, registry *TargetRegistry, coordinator MetricUpdateCoordinatorInterface) *ResourceEventHandler {
	return &ResourceEventHandler{
		logger:            logger.WithName("ResourceEventHandler"),
		targetRegistry:    registry,
		updateCoordinator: coordinator,
	}
}

// OnAdd is called when a resource is added.
func (reh *ResourceEventHandler) OnAdd(obj interface{}, gvk schema.GroupVersionKind) {
	reh.logger.Info("OnAdd event received", "gvk", gvk.String())
	reh.handleEvent(obj, gvk, "add")
}

// OnUpdate is called when a resource is updated.
func (reh *ResourceEventHandler) OnUpdate(oldObj, newObj interface{}, gvk schema.GroupVersionKind) {
	// Check if the resource version has changed to avoid processing no-op updates.
	oldMeta, errOld := meta.Accessor(oldObj)
	if errOld != nil {
		reh.logger.Error(errOld, "Failed to get meta for old object in OnUpdate", "gvk", gvk)
		// Potentially still handle event if oldMeta is not crucial for finding metrics
	}
	newMeta, errNew := meta.Accessor(newObj)
	if errNew != nil {
		reh.logger.Error(errNew, "Failed to get meta for new object in OnUpdate", "gvk", gvk)
		return // Cannot proceed without new object's metadata
	}

	if oldMeta != nil && newMeta.GetResourceVersion() == oldMeta.GetResourceVersion() {
		reh.logger.V(1).Info("Skipping OnUpdate event due to same resource version", "gvk", gvk, "name", newMeta.GetName(), "namespace", newMeta.GetNamespace())
		return
	}

	reh.logger.V(1).Info("OnUpdate event received", "gvk", gvk, "newObject", newObj)
	reh.handleEvent(newObj, gvk, "update")
}

// OnDelete is called when a resource is deleted.
func (reh *ResourceEventHandler) OnDelete(obj interface{}, gvk schema.GroupVersionKind) {
	// Handle cases where the object is a DeletionFinalStateUnknown
	if d, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		reh.logger.Info("OnDelete event received (DeletionFinalStateUnknown)", "gvk", gvk.String(), "key", d.Key)
		obj = d.Obj // Use the actual deleted object
		if obj == nil {
			reh.logger.Info("OnDelete: DeletedFinalStateUnknown contained no object.", "gvk", gvk.String(), "key", d.Key)
			// We might not be able to get labels/namespace here.
			// Consider if metrics need to be refreshed based on key only.
			return
		}
	}
	reh.logger.Info("OnDelete event received", "gvk", gvk.String())
	reh.handleEvent(obj, gvk, "delete")
}

func (reh *ResourceEventHandler) handleEvent(obj interface{}, gvk schema.GroupVersionKind, eventType string) {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		reh.logger.Error(err, "Failed to get metadata accessor for object", "gvk", gvk, "eventType", eventType)
		return
	}

	objNamespace := accessor.GetNamespace()
	objLabels := labels.Set(accessor.GetLabels())

	reh.logger.Info("Handling event",
		"gvk", gvk.String(),
		"namespace", objNamespace,
		"name", accessor.GetName(),
		"labels", objLabels,
		"eventType", eventType,
	)

	// Iterate through all unique targets registered.
	// The TargetRegistry's GetInterestedMetrics might be too specific if we want to match broader criteria.
	// For now, let's assume TargetRegistry can give us relevant metrics.
	// A more efficient approach might be to query TargetRegistry with GVK, namespace, and then filter by label selector.

	// This is a simplified approach. A real implementation needs to efficiently
	// find all Metric CRs whose TargetResourceIdentifier matches the event.
	// This involves checking GVK, Namespace (if applicable), and LabelSelector.

	registeredTargets := reh.targetRegistry.GetUniqueTargets()
	for _, registeredTarget := range registeredTargets {
		if registeredTarget.GVK != gvk {
			continue
		}

		// Namespace check:
		// The registeredTarget.Namespace contains the namespace of the Metric CR, not necessarily
		// the namespace we want to watch. For namespaced resources like Pods, we want to watch
		// resources in the same namespace as the Metric CR.
		// For cluster-scoped resources like Nodes, objNamespace will be empty.

		// Check if this is a namespaced resource that needs namespace matching
		if objNamespace != "" && registeredTarget.Namespace != objNamespace {
			// For namespaced resources, the metric should watch resources in its own namespace
			// registeredTarget.Namespace is the namespace of the Metric CR
			// objNamespace is the namespace of the resource event
			// They should match for the metric to be interested in this event
			reh.logger.V(2).Info("Skipping event due to namespace mismatch",
				"registeredTargetNamespace", registeredTarget.Namespace,
				"objNamespace", objNamespace,
				"gvk", gvk.String(),
				"objName", accessor.GetName(),
			)
			continue
		}
		// For cluster-scoped resources (objNamespace is empty), we don't need namespace matching
		// The metric can be in any namespace and watch cluster-scoped resources

		if !registeredTarget.Selector.Matches(objLabels) {
			continue
		}

		// If all checks pass, this registeredTarget is relevant.
		// Now find all Metric CRs associated with this specific registeredTarget.
		// (Note: GetUniqueTargets gives one instance, but multiple metrics might share this exact target spec)
		// We need a way to get all metrics for this *specific* registeredTarget.
		// The current GetInterestedMetrics is what we need here.

		interestedMetricKeys := reh.targetRegistry.GetInterestedMetrics(registeredTarget)

		for _, metricKey := range interestedMetricKeys {
			reh.logger.Info("Metric is interested in this event",
				"metric", metricKey.String(),
				"targetGVK", registeredTarget.GVK.String(),
				"targetNamespace", registeredTarget.Namespace,
				"targetSelector", registeredTarget.Selector.String(),
				"eventObjName", accessor.GetName(),
			)
			if reh.updateCoordinator != nil {
				// Pass the actual event object and its GVK
				reh.updateCoordinator.RequestMetricUpdate(metricKey.String(), gvk, obj)
			}
		}
	}
}

package controller

import (
	"sync"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/SAP/metrics-operator/api/v1alpha1"
)

// TargetResourceIdentifier defines the unique key for a watched resource type.
// It includes GVK, and optionally namespace and selector for more specific watches.
type TargetResourceIdentifier struct {
	GVK       schema.GroupVersionKind
	Namespace string // Empty for cluster-scoped resources or all-namespace watches
	Selector  labels.Selector
}

// MetricInterest holds information about a Metric CR's interest in a target resource.
type MetricInterest struct {
	MetricKey types.NamespacedName
	Target    TargetResourceIdentifier
	// TODO: Add RemoteClusterAccessRef if needed for multi-cluster informer management directly here
}

// TargetRegistry keeps track of which Metric CRs are interested in which target Kubernetes resources.
type TargetRegistry struct {
	mu sync.RWMutex
	// interests maps a Metric's NamespacedName to its specific target interest.
	interests map[types.NamespacedName]MetricInterest
	// targetToMetrics maps a simplified TargetResourceIdentifier (e.g., GVK only or GVK+Namespace)
	// to a set of MetricKeys interested in it. This helps quickly find relevant metrics for an event.
	// For simplicity, we might start with GVK to set of metric keys.
	// A more complex key might be needed for efficient event routing.
	// For now, GetUniqueTargets will iterate `interests`.
}

// NewTargetRegistry creates a new TargetRegistry.
func NewTargetRegistry() *TargetRegistry {
	return &TargetRegistry{
		interests: make(map[types.NamespacedName]MetricInterest),
	}
}

// Register records a Metric's interest in a target resource.
// It extracts target information from the Metric spec.
func (r *TargetRegistry) Register(metric *v1alpha1.Metric) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// metric.Spec.Target is a GroupVersionKind struct, not a pointer.
	// It's required by CRD validation, so not checking for nil.
	// If Kind is empty, it's an invalid spec for event-driven, but registration can proceed.
	// The DynamicInformerManager should handle invalid GVKs.

	gvk := schema.GroupVersionKind{
		Group:   metric.Spec.Target.Group,
		Version: metric.Spec.Target.Version,
		Kind:    metric.Spec.Target.Kind,
	}

	var selector labels.Selector = labels.Everything()
	if metric.Spec.LabelSelector != "" {
		sel, err := labels.Parse(metric.Spec.LabelSelector)
		if err != nil {
			// Consider logging this error and potentially returning it or using labels.Everything()
			// For now, let's return the error to make it explicit.
			return err
		}
		selector = sel
	}
	// TODO: Handle FieldSelector if needed by informers (metric.Spec.FieldSelector)

	metricKey := types.NamespacedName{Name: metric.Name, Namespace: metric.Namespace}
	r.interests[metricKey] = MetricInterest{
		MetricKey: metricKey,
		Target: TargetResourceIdentifier{
			GVK: gvk,
			// Use the Metric CR's namespace as the target namespace.
			// The DynamicInformerManager can decide to watch all namespaces
			// if the GVK is cluster-scoped or if metric.Namespace is empty (for a ClusterMetric CR if it existed).
			Namespace: metric.Namespace,
			Selector:  selector,
		},
	}
	// TODO: Update reverse lookup maps if implemented (e.g., targetToMetrics)
	return nil
}

// Unregister removes a Metric's interest from the registry.
func (r *TargetRegistry) Unregister(metricKey types.NamespacedName) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.interests, metricKey)
	// TODO: Update reverse lookup maps if implemented
}

// GetUniqueTargets returns a list of unique TargetResourceIdentifiers that need informers.
// This helps the DynamicInformerManager know what to watch.
func (r *TargetRegistry) GetUniqueTargets() []TargetResourceIdentifier {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Use a slice to track unique targets since TargetResourceIdentifier contains
	// a labels.Selector which is not hashable and cannot be used as a map key
	var uniqueTargetsList []TargetResourceIdentifier

	for _, interest := range r.interests {
		// Check if this target already exists in our list
		found := false
		for _, existing := range uniqueTargetsList {
			if existing.GVK == interest.Target.GVK &&
				existing.Namespace == interest.Target.Namespace &&
				existing.Selector.String() == interest.Target.Selector.String() {
				found = true
				break
			}
		}

		if !found {
			uniqueTargetsList = append(uniqueTargetsList, interest.Target)
		}
	}

	return uniqueTargetsList
}

// GetInterestedMetrics returns a list of MetricKeys interested in a given target.
// This is a simple version; a more optimized version might use pre-built reverse maps.
func (r *TargetRegistry) GetInterestedMetrics(target TargetResourceIdentifier) []types.NamespacedName {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var interestedMetrics []types.NamespacedName
	for key, interest := range r.interests {
		// Exact match for now. More sophisticated matching might be needed
		// (e.g., if target.Namespace is empty, match all namespaces for that GVK).
		if interest.Target.GVK == target.GVK &&
			interest.Target.Namespace == target.Namespace &&
			interest.Target.Selector.String() == target.Selector.String() { // Selector comparison
			interestedMetrics = append(interestedMetrics, key)
		}
	}
	return interestedMetrics
}

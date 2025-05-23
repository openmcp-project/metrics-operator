package controller

import (
	"context"
	"fmt"
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

	// scopeDiscovery provides dynamic discovery of resource scope (cluster-scoped vs namespaced)
	scopeDiscovery *ResourceScopeDiscovery
}

// NewTargetRegistry creates a new TargetRegistry.
func NewTargetRegistry(scopeDiscovery *ResourceScopeDiscovery) *TargetRegistry {
	return &TargetRegistry{
		interests:      make(map[types.NamespacedName]MetricInterest),
		scopeDiscovery: scopeDiscovery,
	}
}

// Register records a Metric's interest in a target resource.
// It extracts target information from the Metric spec.
func (r *TargetRegistry) Register(ctx context.Context, metric *v1alpha1.Metric) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	gvk := schema.GroupVersionKind{
		Group:   metric.Spec.Target.Group,
		Version: metric.Spec.Target.Version,
		Kind:    metric.Spec.Target.Kind,
	}

	var selector labels.Selector = labels.Everything()
	if metric.Spec.LabelSelector != "" {
		sel, err := labels.Parse(metric.Spec.LabelSelector)
		if err != nil {
			return err
		}
		selector = sel
	}

	metricKey := types.NamespacedName{Name: metric.Name, Namespace: metric.Namespace}

	targetNamespace := metric.Namespace // Default for namespaced resources
	isClusterScoped := false
	if r.scopeDiscovery != nil {
		var err error
		isClusterScoped, err = r.scopeDiscovery.IsClusterScoped(ctx, gvk)
		if err != nil {
			fmt.Printf("[TargetRegistry] Error discovering scope for GVK %s: %v. Will retry discovery later.\n", gvk.String(), err)
			// For now, we'll proceed with the default (namespaced) but this will be retried
			// when the informer manager attempts to create the informer
		}
		if isClusterScoped {
			targetNamespace = ""
		}
	}
	fmt.Printf("[TargetRegistry] Registering metric %s for GVK %s: isClusterScoped=%v, targetNamespace='%s'\n",
		metricKey.String(), gvk.String(), isClusterScoped, targetNamespace)

	r.interests[metricKey] = MetricInterest{
		MetricKey: metricKey,
		Target: TargetResourceIdentifier{
			GVK:       gvk,
			Namespace: targetNamespace,
			Selector:  selector,
		},
	}
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

package controller

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// ResourceScopeDiscovery provides dynamic discovery of whether Kubernetes resources are cluster-scoped or namespaced.
// It uses the Kubernetes Discovery API to query resource metadata and caches results for performance.
type ResourceScopeDiscovery struct {
	discoveryClient discovery.DiscoveryInterface
	cache           map[schema.GroupVersionKind]bool
	cacheMutex      sync.RWMutex
	logger          logr.Logger
}

// NewResourceScopeDiscovery creates a new ResourceScopeDiscovery instance.
func NewResourceScopeDiscovery(discoveryClient discovery.DiscoveryInterface, logger logr.Logger) *ResourceScopeDiscovery {
	return &ResourceScopeDiscovery{
		discoveryClient: discoveryClient,
		cache:           make(map[schema.GroupVersionKind]bool),
		logger:          logger.WithName("ResourceScopeDiscovery"),
	}
}

// IsClusterScoped determines if a given GVK represents a cluster-scoped resource by querying the Kubernetes API.
// It returns true for cluster-scoped resources, false for namespaced resources.
// If discovery fails, it returns an error rather than defaulting to avoid incorrect assumptions.
func (rsd *ResourceScopeDiscovery) IsClusterScoped(ctx context.Context, gvk schema.GroupVersionKind) (bool, error) {
	// Check cache first
	if cached, found := rsd.getFromCache(gvk); found {
		rsd.logger.V(2).Info("Using cached resource scope", "gvk", gvk, "clusterScoped", cached)
		return cached, nil
	}

	// Query API server using discovery client with improved robustness
	isClusterScoped, err := rsd.discoverResourceScope(ctx, gvk)
	if err != nil {
		fmt.Printf("[ResourceScopeDiscovery] Failed to discover resource scope for GVK %s: %v\n", gvk.String(), err)
		rsd.logger.V(1).Info("Failed to discover resource scope",
			"gvk", gvk, "error", err)
		return false, err
	}

	rsd.setCache(gvk, isClusterScoped)
	fmt.Printf("[ResourceScopeDiscovery] Discovered resource scope for GVK %s: clusterScoped=%v\n", gvk.String(), isClusterScoped)
	rsd.logger.V(1).Info("Discovered resource scope", "gvk", gvk, "clusterScoped", isClusterScoped)
	return isClusterScoped, nil
}

// getFromCache safely retrieves a cached result.
func (rsd *ResourceScopeDiscovery) getFromCache(gvk schema.GroupVersionKind) (bool, bool) {
	rsd.cacheMutex.RLock()
	defer rsd.cacheMutex.RUnlock()
	value, found := rsd.cache[gvk]
	return value, found
}

// setCache safely stores a result in the cache.
func (rsd *ResourceScopeDiscovery) setCache(gvk schema.GroupVersionKind, isClusterScoped bool) {
	rsd.cacheMutex.Lock()
	defer rsd.cacheMutex.Unlock()
	rsd.cache[gvk] = isClusterScoped
}

// discoverResourceScope queries the Kubernetes API to determine if a resource is cluster-scoped.
// Uses a robust approach with multiple discovery methods for better reliability.
func (rsd *ResourceScopeDiscovery) discoverResourceScope(ctx context.Context, gvk schema.GroupVersionKind) (bool, error) {
	rsd.logger.V(3).Info("Starting discovery for GVK", "gvk", gvk)

	// Try the more robust ServerPreferredResources first
	if isClusterScoped, err := rsd.discoverUsingPreferredResources(ctx, gvk); err == nil {
		rsd.logger.V(3).Info("Successfully discovered scope using preferred resources", "gvk", gvk, "clusterScoped", isClusterScoped)
		rsd.logger.V(2).Info("Successfully discovered scope using preferred resources", "gvk", gvk, "clusterScoped", isClusterScoped)
		return isClusterScoped, nil
	}

	rsd.logger.V(3).Info("Preferred resources discovery failed, trying group/version method", "gvk", gvk)
	rsd.logger.V(2).Info("Preferred resources discovery failed, trying group/version method", "gvk", gvk)

	// Fallback to the original group/version specific method
	rsd.logger.V(3).Info("Trying group/version discovery for GVK", "gvk", gvk)
	return rsd.discoverUsingGroupVersion(ctx, gvk)
}

// discoverUsingPreferredResources uses ServerPreferredResources for more robust discovery.
// This method gets all preferred resources across all API groups in one call, which is more
// reliable than querying specific group/versions that might not be ready yet.
func (rsd *ResourceScopeDiscovery) discoverUsingPreferredResources(_ context.Context, gvk schema.GroupVersionKind) (bool, error) {
	// Get all preferred resources across all API groups
	resourceLists, err := rsd.discoveryClient.ServerPreferredResources()
	if err != nil {
		return false, fmt.Errorf("failed to get preferred resources: %w", err)
	}

	// Search through all resource lists for our GVK
	targetGroupVersion := gvk.GroupVersion().String()
	for _, resourceList := range resourceLists {
		if resourceList.GroupVersion == targetGroupVersion {
			for _, apiResource := range resourceList.APIResources {
				if apiResource.Kind == gvk.Kind {
					// The Namespaced field indicates if the resource is namespaced
					// If Namespaced is true, the resource is namespaced (not cluster-scoped)
					// If Namespaced is false, the resource is cluster-scoped
					isClusterScoped := !apiResource.Namespaced
					return isClusterScoped, nil
				}
			}
		}
	}

	return false, fmt.Errorf("resource kind %s not found in preferred resources for %s", gvk.Kind, targetGroupVersion)
}

// discoverUsingGroupVersion uses the original ServerResourcesForGroupVersion method as fallback.
func (rsd *ResourceScopeDiscovery) discoverUsingGroupVersion(_ context.Context, gvk schema.GroupVersionKind) (bool, error) {
	// Get the API resources for the group/version
	groupVersion := gvk.GroupVersion().String()
	apiResourceList, err := rsd.discoveryClient.ServerResourcesForGroupVersion(groupVersion)
	if err != nil {
		return false, fmt.Errorf("failed to get server resources for %s: %w", groupVersion, err)
	}

	// Find the specific resource by kind
	for _, apiResource := range apiResourceList.APIResources {
		if apiResource.Kind == gvk.Kind {
			// The Namespaced field indicates if the resource is namespaced
			// If Namespaced is true, the resource is namespaced (not cluster-scoped)
			// If Namespaced is false, the resource is cluster-scoped
			isClusterScoped := !apiResource.Namespaced
			return isClusterScoped, nil
		}
	}

	return false, fmt.Errorf("resource kind %s not found in group/version %s", gvk.Kind, groupVersion)
}

// ClearCache clears the internal cache. Useful for testing or when resource definitions change.
func (rsd *ResourceScopeDiscovery) ClearCache() {
	rsd.cacheMutex.Lock()
	defer rsd.cacheMutex.Unlock()
	rsd.cache = make(map[schema.GroupVersionKind]bool)
	rsd.logger.V(1).Info("Resource scope cache cleared")
}

// GetCacheSize returns the current number of cached entries. Useful for monitoring and testing.
func (rsd *ResourceScopeDiscovery) GetCacheSize() int {
	rsd.cacheMutex.RLock()
	defer rsd.cacheMutex.RUnlock()
	return len(rsd.cache)
}

package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// GVRDiscoveryService provides dynamic discovery of GroupVersionResource (GVR) from GroupVersionKind (GVK).
// It uses the Kubernetes Discovery API to query resource metadata and caches results for performance.
type GVRDiscoveryService struct {
	discoveryClient discovery.DiscoveryInterface
	cache           map[schema.GroupVersionKind]schema.GroupVersionResource
	cacheMutex      sync.RWMutex
	logger          logr.Logger
}

// NewGVRDiscoveryService creates a new GVRDiscoveryService instance.
func NewGVRDiscoveryService(discoveryClient discovery.DiscoveryInterface, logger logr.Logger) *GVRDiscoveryService {
	return &GVRDiscoveryService{
		discoveryClient: discoveryClient,
		cache:           make(map[schema.GroupVersionKind]schema.GroupVersionResource),
		logger:          logger.WithName("GVRDiscoveryService"),
	}
}

// GetGVR converts a GroupVersionKind to GroupVersionResource by querying the Kubernetes API.
// It returns the correct resource name for the given kind, with caching for performance.
// If discovery fails, it returns an error since GVR is required for informer creation.
func (gds *GVRDiscoveryService) GetGVR(ctx context.Context, gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	// Check cache first
	if cached, found := gds.getFromCache(gvk); found {
		gds.logger.V(2).Info("Using cached GVR", "gvk", gvk, "gvr", cached)
		return cached, nil
	}

	// Query API server using discovery client
	gvr, err := gds.discoverGVR(ctx, gvk)
	if err != nil {
		gds.logger.V(1).Info("Failed to discover GVR", "gvk", gvk, "error", err)
		// Don't cache failed discoveries to allow retry
		return schema.GroupVersionResource{}, err
	}

	// Cache the successful result
	gds.setCache(gvk, gvr)
	gds.logger.V(1).Info("Discovered GVR", "gvk", gvk, "gvr", gvr)
	return gvr, nil
}

// getFromCache safely retrieves a cached result.
func (gds *GVRDiscoveryService) getFromCache(gvk schema.GroupVersionKind) (schema.GroupVersionResource, bool) {
	gds.cacheMutex.RLock()
	defer gds.cacheMutex.RUnlock()
	value, found := gds.cache[gvk]
	return value, found
}

// setCache safely stores a result in the cache.
func (gds *GVRDiscoveryService) setCache(gvk schema.GroupVersionKind, gvr schema.GroupVersionResource) {
	gds.cacheMutex.Lock()
	defer gds.cacheMutex.Unlock()
	gds.cache[gvk] = gvr
}

// discoverGVR queries the Kubernetes API to find the resource name for a given kind.
// This is based on the existing GetGVRfromGVK function in metrichandler.go but with improved error handling.
func (gds *GVRDiscoveryService) discoverGVR(_ context.Context, gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	// Get the API resources for the group/version
	groupVersion := gvk.GroupVersion().String()
	apiResourceList, err := gds.discoveryClient.ServerResourcesForGroupVersion(groupVersion)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("failed to get server resources for %s: %w", groupVersion, err)
	}

	// Find the specific resource by kind (case-insensitive matching like the original)
	for _, apiResource := range apiResourceList.APIResources {
		if strings.EqualFold(apiResource.Kind, gvk.Kind) {
			return schema.GroupVersionResource{
				Group:    gvk.Group,
				Version:  gvk.Version,
				Resource: apiResource.Name,
			}, nil
		}
	}

	return schema.GroupVersionResource{}, fmt.Errorf("resource kind %s not found in group/version %s", gvk.Kind, groupVersion)
}

// ClearCache clears the internal cache. Useful for testing or when resource definitions change.
func (gds *GVRDiscoveryService) ClearCache() {
	gds.cacheMutex.Lock()
	defer gds.cacheMutex.Unlock()
	gds.cache = make(map[schema.GroupVersionKind]schema.GroupVersionResource)
	gds.logger.V(1).Info("GVR cache cleared")
}

// GetCacheSize returns the current number of cached entries. Useful for monitoring and testing.
func (gds *GVRDiscoveryService) GetCacheSize() int {
	gds.cacheMutex.RLock()
	defer gds.cacheMutex.RUnlock()
	return len(gds.cache)
}

package orchestrator

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
)

const defaultAutoDiscoveryCacheThreshold = 500 * time.Millisecond

type targetResource struct {
	GVR schema.GroupVersionResource
	GVK schema.GroupVersionKind
}

type targetLookupResult struct {
	Resources []targetResource
	Duration  time.Duration
	CacheHits int64
}

type discoveryCacheEntry struct {
	Resources    []targetResource
	ExpiresAt    time.Time
	LastDuration time.Duration
}

type DiscoveryResource interface {
	GetTarget() *v1alpha1.GroupVersionKindTarget
	GetMetricObservation() *v1alpha1.MetricObservation
	SetMetricObservation(*v1alpha1.MetricObservation)
}

var discoveryLookupCache = struct {
	sync.Mutex
	entries map[string]discoveryCacheEntry
}{entries: map[string]discoveryCacheEntry{}}

func discoveryCacheKeyFromConfig(qc QueryConfig) string {
	cluster := ""
	if qc.ClusterName != nil {
		cluster = *qc.ClusterName
	}
	return fmt.Sprintf("%s|%s", qc.RestConfig.Host, cluster)
}

func lookupTargetResources(target v1alpha1.GroupVersionKindTarget, observation v1alpha1.CommonObservation, disco discovery.DiscoveryInterface, clusterKey string) (targetLookupResult, error) {
	key := targetDiscoveryCacheKey(clusterKey, target.GVK())

	if cacheEnabledOrAutoAndSlowDiscovery(target, observation) {
		if resources, expired := getDiscoveryCache(key); !expired {
			return targetLookupResult{Resources: resources, CacheHits: 1, Duration: time.Duration(observation.TargetLookupDurationMillis) * time.Millisecond}, nil
		}
	}

	start := time.Now()
	resources, err := discoverTargetResources(target.GVK(), disco)
	duration := time.Since(start)
	if err != nil {
		return targetLookupResult{Duration: duration}, err
	}

	setDiscoveryCache(key, resources, target.Cache.TTL(), duration)

	return targetLookupResult{Resources: resources, Duration: duration}, nil
}

func cacheEnabledOrAutoAndSlowDiscovery(target v1alpha1.GroupVersionKindTarget, observation v1alpha1.CommonObservation) bool {
	cacheMode := target.Cache.Mode()
	if cacheMode == v1alpha1.TargetCacheEnabledTrue {
		return true
	}
	if observation.TargetLookupDurationMillis > defaultAutoDiscoveryCacheThreshold.Milliseconds() {
		return true
	}
	return false
}

func getDiscoveryCache(key string) ([]targetResource, bool) {
	discoveryLookupCache.Lock()
	defer discoveryLookupCache.Unlock()

	entry, ok := discoveryLookupCache.entries[key]
	if !ok || time.Now().After(entry.ExpiresAt) {
		if ok {
			delete(discoveryLookupCache.entries, key)
		}
		return nil, true
	}
	return append([]targetResource(nil), entry.Resources...), false
}

func setDiscoveryCache(key string, resources []targetResource, ttl time.Duration, duration time.Duration) {
	discoveryLookupCache.Lock()
	defer discoveryLookupCache.Unlock()
	discoveryLookupCache.entries[key] = discoveryCacheEntry{
		Resources:    append([]targetResource(nil), resources...),
		ExpiresAt:    time.Now().Add(ttl),
		LastDuration: duration,
	}
}

func targetDiscoveryCacheKey(clusterKey string, gvk schema.GroupVersionKind) string {
	return fmt.Sprintf("%s|%s|%s|%s", clusterKey, gvk.Group, gvk.Version, gvk.Kind)
}

func discoverTargetResources(gvk schema.GroupVersionKind, disco discovery.DiscoveryInterface) ([]targetResource, error) {
	if gvk.Group == "" && gvk.Version == "" && gvk.Kind == "" {
		return nil, fmt.Errorf("target must set at least one of group, version or kind")
	}

	if gvk.Group != "" && gvk.Version != "" && gvk.Kind != "" {
		gvr, err := GetGVRfromGVK(gvk, disco)
		if err != nil {
			return nil, err
		}
		return []targetResource{{GVR: gvr, GVK: gvk}}, nil
	}

	groupVersions, err := matchingGroupVersions(gvk, disco)
	if err != nil {
		return nil, err
	}

	resources := make([]targetResource, 0)
	for _, gv := range groupVersions {
		resourceList, err := disco.ServerResourcesForGroupVersion(gv.String())
		if err != nil {
			return nil, fmt.Errorf("failed to discover resources for %s: %w", gv.String(), err)
		}
		for _, resource := range resourceList.APIResources {
			if !isListableTopLevelResource(resource) {
				continue
			}
			if gvk.Kind != "" && !strings.EqualFold(gvk.Kind, resource.Kind) {
				continue
			}
			resources = append(resources, targetResource{
				GVR: schema.GroupVersionResource{Group: gv.Group, Version: gv.Version, Resource: resource.Name},
				GVK: schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: resource.Kind},
			})
		}
	}

	sort.Slice(resources, func(i, j int) bool {
		return resources[i].GVR.String() < resources[j].GVR.String()
	})

	if len(resources) == 0 {
		return nil, fmt.Errorf("no resources found for target filter %s", gvk.String())
	}
	return resources, nil
}

func matchingGroupVersions(gvk schema.GroupVersionKind, disco discovery.DiscoveryInterface) ([]schema.GroupVersion, error) {
	if gvk.Group != "" && gvk.Version != "" {
		return []schema.GroupVersion{{Group: gvk.Group, Version: gvk.Version}}, nil
	}

	groups, err := disco.ServerGroups()
	if err != nil {
		return nil, err
	}

	groupVersions := make([]schema.GroupVersion, 0)
	for _, group := range groups.Groups {
		if gvk.Group != "" && group.Name != gvk.Group {
			continue
		}
		for _, version := range group.Versions {
			gv, err := schema.ParseGroupVersion(version.GroupVersion)
			if err != nil {
				return nil, err
			}
			if gvk.Version != "" && gv.Version != gvk.Version {
				continue
			}
			groupVersions = append(groupVersions, gv)
		}
	}

	if len(groupVersions) == 0 {
		return nil, fmt.Errorf("no API group versions found for target filter %s", gvk.String())
	}
	return groupVersions, nil
}

func isListableTopLevelResource(resource metav1.APIResource) bool {
	if strings.Contains(resource.Name, "/") {
		return false
	}
	if len(resource.Verbs) == 0 {
		return true
	}
	for _, verb := range resource.Verbs {
		if verb == "list" {
			return true
		}
	}
	return false
}

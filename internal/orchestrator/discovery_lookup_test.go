package orchestrator

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
)

func TestDiscoverTargetResourcesPartialGVK(t *testing.T) {
	disco := &fake.FakeDiscovery{Fake: &clienttesting.Fake{}}
	disco.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", Kind: "Deployment", Verbs: []string{"get", "list"}},
				{Name: "deployments/status", Kind: "Deployment", Verbs: []string{"get"}},
			},
		},
		{
			GroupVersion: "batch/v1",
			APIResources: []metav1.APIResource{
				{Name: "jobs", Kind: "Job", Verbs: []string{"get", "list"}},
			},
		},
		{
			GroupVersion: "example.io/v1alpha1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", Kind: "Deployment", Verbs: []string{"list"}},
			},
		},
	}

	tests := []struct {
		name string
		gvk  schema.GroupVersionKind
		want []schema.GroupVersionResource
	}{
		{
			name: "full GVK",
			gvk:  schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			want: []schema.GroupVersionResource{{Group: "apps", Version: "v1", Resource: "deployments"}},
		},
		{
			name: "group only",
			gvk:  schema.GroupVersionKind{Group: "batch"},
			want: []schema.GroupVersionResource{{Group: "batch", Version: "v1", Resource: "jobs"}},
		},
		{
			name: "version only",
			gvk:  schema.GroupVersionKind{Version: "v1"},
			want: []schema.GroupVersionResource{{Group: "apps", Version: "v1", Resource: "deployments"}, {Group: "batch", Version: "v1", Resource: "jobs"}},
		},
		{
			name: "kind only",
			gvk:  schema.GroupVersionKind{Kind: "Deployment"},
			want: []schema.GroupVersionResource{{Group: "apps", Version: "v1", Resource: "deployments"}, {Group: "example.io", Version: "v1alpha1", Resource: "deployments"}},
		},
		{
			name: "group and version",
			gvk:  schema.GroupVersionKind{Group: "apps", Version: "v1"},
			want: []schema.GroupVersionResource{{Group: "apps", Version: "v1", Resource: "deployments"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := discoverTargetResources(tt.gvk, disco)
			require.NoError(t, err)
			gotGVRs := make([]schema.GroupVersionResource, 0, len(got))
			for _, resource := range got {
				gotGVRs = append(gotGVRs, resource.GVR)
			}
			require.ElementsMatch(t, tt.want, gotGVRs)
		})
	}
}

func TestGetDiscoveryCacheReadPaths(t *testing.T) {
	resetDiscoveryLookupCache()

	resources := []targetResource{{
		GVR: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		GVK: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
	}}

	tests := []struct {
		name        string
		expiresAt   time.Time
		wantExpired bool
	}{
		{
			name:        "valid cache hits",
			expiresAt:   time.Now().Add(time.Minute),
			wantExpired: false,
		},
		{
			name:        "expired cache misses",
			expiresAt:   time.Now().Add(-time.Minute),
			wantExpired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetDiscoveryLookupCache()
			discoveryLookupCache.Lock()
			discoveryLookupCache.entries["key"] = discoveryCacheEntry{
				Resources: resources,
				ExpiresAt: tt.expiresAt,
			}
			discoveryLookupCache.Unlock()

			got, expired := getDiscoveryCache("key")
			require.Equal(t, tt.wantExpired, expired)
			if tt.wantExpired {
				require.Nil(t, got)
			} else {
				require.Equal(t, resources, got)
			}
		})
	}
}

func TestLookupTargetResourcesCacheWritePaths(t *testing.T) {
	tests := []struct {
		name       string
		mode       v1alpha1.TargetCacheEnabled
		wantWrite  bool
		wantReadOn v1alpha1.TargetCacheEnabled
		wantHit    bool
	}{
		{
			name:       "enabled true writes cache and reads it",
			mode:       v1alpha1.TargetCacheEnabledTrue,
			wantWrite:  true,
			wantReadOn: v1alpha1.TargetCacheEnabledTrue,
			wantHit:    true,
		},
		{
			name:       "enabled false writes refresh result but does not read it",
			mode:       v1alpha1.TargetCacheEnabledFalse,
			wantWrite:  true,
			wantReadOn: v1alpha1.TargetCacheEnabledFalse,
			wantHit:    false,
		},
		{
			name:       "auto writes refresh metadata but does not read fast cache",
			mode:       v1alpha1.TargetCacheEnabledAuto,
			wantWrite:  true,
			wantReadOn: v1alpha1.TargetCacheEnabledAuto,
			wantHit:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetDiscoveryLookupCache()
			disco := fakeDiscoveryForDeployment()
			target := v1alpha1.GroupVersionKindTarget{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
				Cache:   v1alpha1.TargetCache{Enabled: tt.mode, TTLInMinutes: 1},
			}
			key := targetDiscoveryCacheKey("test-cluster", target.GVK())

			result, err := lookupTargetResources(target, v1alpha1.CommonObservation{}, disco, "test-cluster")
			require.NoError(t, err)
			require.Zero(t, result.CacheHits)
			require.Positive(t, result.Duration)

			discoveryLookupCache.Lock()
			entry, written := discoveryLookupCache.entries[key]
			discoveryLookupCache.Unlock()
			require.Equal(t, tt.wantWrite, written)
			if !tt.wantWrite {
				return
			}
			require.Equal(t, result.Duration, entry.LastDuration)
			require.NotEmpty(t, entry.Resources)
			require.True(t, entry.ExpiresAt.After(time.Now()))

			observation := v1alpha1.CommonObservation{}
			if tt.wantReadOn == v1alpha1.TargetCacheEnabledAuto {
				observation.TargetLookupDurationMillis = defaultAutoDiscoveryCacheThreshold.Milliseconds()
			}
			require.Equal(t, tt.wantHit, cacheEnabledOrAutoAndSlowDiscovery(target, observation))
		})
	}
}

func TestLookupTargetResourcesAutoReadsCacheAfterSlowRefresh(t *testing.T) {
	resetDiscoveryLookupCache()
	disco := fakeDiscoveryForDeployment()
	target := v1alpha1.GroupVersionKindTarget{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
		Cache:   v1alpha1.TargetCache{Enabled: v1alpha1.TargetCacheEnabledAuto, TTLInMinutes: 1},
	}
	key := targetDiscoveryCacheKey("test-cluster", target.GVK())

	setDiscoveryCache(key, []targetResource{{
		GVR: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		GVK: target.GVK(),
	}}, time.Minute, defaultAutoDiscoveryCacheThreshold+time.Millisecond)

	result, err := lookupTargetResources(target, v1alpha1.CommonObservation{TargetLookupDurationMillis: defaultAutoDiscoveryCacheThreshold.Milliseconds() + 1}, disco, "test-cluster")
	require.NoError(t, err)
	require.EqualValues(t, 1, result.CacheHits)
	require.Equal(t, defaultAutoDiscoveryCacheThreshold+time.Millisecond, result.Duration)
}

func resetDiscoveryLookupCache() {
	discoveryLookupCache.Lock()
	defer discoveryLookupCache.Unlock()
	discoveryLookupCache.entries = map[string]discoveryCacheEntry{}
}

func fakeDiscoveryForDeployment() *fake.FakeDiscovery {
	disco := &fake.FakeDiscovery{Fake: &clienttesting.Fake{}}
	disco.Resources = []*metav1.APIResourceList{{
		GroupVersion: "apps/v1",
		APIResources: []metav1.APIResource{{Name: "deployments", Kind: "Deployment", Verbs: []string{"list"}}},
	}}
	return disco
}

func TestTargetCacheDefaults(t *testing.T) {
	var cache v1alpha1.TargetCache
	require.Equal(t, v1alpha1.TargetCacheEnabledAuto, cache.Mode())
	require.Equal(t, 10*time.Minute, cache.TTL())
}

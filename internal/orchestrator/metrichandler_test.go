package orchestrator

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
)

func fakeDiscoveryForGVK(group, version, kind, plural string) *fake.FakeDiscovery {
	disco := &fake.FakeDiscovery{Fake: &clienttesting.Fake{}}
	disco.Resources = []*metav1.APIResourceList{{
		GroupVersion: group + "/" + version,
		APIResources: []metav1.APIResource{{Name: plural, Kind: kind, Verbs: []string{"list"}}},
	}}
	return disco
}

func nopGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "nop.crossplane.io",
		Version: "v1alpha1",
		Kind:    "NopResource",
	}
}

func newMetricHandlerForTest(t *testing.T, gvk schema.GroupVersionKind, resources []string, projections []v1alpha1.Projection) (*MetricHandler, map[string]int64) {
	t.Helper()
	gauge := newTestGauge(t)
	recorded := make(map[string]int64)
	gauge.SetPrometheusFunc(func(dims map[string]string, value int64) error {
		// key is all dims joined; caller can inspect as needed
		key := dims[RESOURCE] + "|" + dims[GROUP] + "|" + dims[VERSION]
		for _, p := range projections {
			key += "|" + p.Name + "=" + dims[p.Name]
		}
		recorded[key] = value
		return nil
	})
	handler := &MetricHandler{
		dCli:        setupFakeDynamicClient(t, resources),
		discoClient: fakeDiscoveryForGVK(gvk.Group, gvk.Version, gvk.Kind, "nopresources"),
		metric: v1alpha1.Metric{
			Spec: v1alpha1.MetricSpec{
				Target: v1alpha1.GroupVersionKindTarget{
					Group:   gvk.Group,
					Version: gvk.Version,
					Kind:    gvk.Kind,
				},
				Projections: projections,
			},
		},
		gaugeMetric: gauge,
	}
	return handler, recorded
}

func TestMetricHandlerSimpleMonitor(t *testing.T) {
	resetDiscoveryLookupCache()
	gvk := nopGVK()

	handler, recorded := newMetricHandlerForTest(t, gvk, []string{fakeResource(gvk), fakeResource(gvk)}, nil)

	result, err := handler.Monitor(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != v1alpha1.PhaseActive {
		t.Fatalf("unexpected phase: wanted=%s, got=%s", v1alpha1.PhaseActive, result.Phase)
	}
	key := gvk.Kind + "|" + gvk.Group + "|" + gvk.Version
	if recorded[key] != 2 {
		t.Fatalf("unexpected count: wanted=2, got=%d (records=%v)", recorded[key], recorded)
	}
}

func TestMetricHandlerSimpleMonitorSingleResource(t *testing.T) {
	resetDiscoveryLookupCache()
	gvk := nopGVK()

	handler, recorded := newMetricHandlerForTest(t, gvk, []string{fakeResource(gvk)}, nil)

	result, err := handler.Monitor(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != v1alpha1.PhaseActive {
		t.Fatalf("unexpected phase: %s", result.Phase)
	}
	key := gvk.Kind + "|" + gvk.Group + "|" + gvk.Version
	if recorded[key] != 1 {
		t.Fatalf("unexpected value: wanted=1, got=%d (records=%v)", recorded[key], recorded)
	}
}

func TestMetricHandlerProjectionsMonitor(t *testing.T) {
	resetDiscoveryLookupCache()
	gvk := nopGVK()

	// fakeResource produces resources with status.conditions where Ready=True
	projections := []v1alpha1.Projection{
		{Name: "ready", FieldPath: "status.conditions[?(@.type=='Ready')].status", Type: v1alpha1.TypePrimitive},
	}
	handler, recorded := newMetricHandlerForTest(t, gvk,
		[]string{fakeResource(gvk), fakeResource(gvk)},
		projections)

	result, err := handler.Monitor(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != v1alpha1.PhaseActive {
		t.Fatalf("unexpected phase: %s, err=%v", result.Phase, result.Error)
	}
	// Both resources have Ready=True → one bucket with count=2
	key := gvk.Kind + "|" + gvk.Group + "|" + gvk.Version + "|ready=True"
	if recorded[key] != 2 {
		t.Fatalf("unexpected projection bucket: wanted %q=2, got=%v", key, recorded)
	}
}

func TestMetricHandlerGetResourcesFailed(t *testing.T) {
	resetDiscoveryLookupCache()
	gvk := nopGVK()

	// empty discovery → lookupTargetResources fails
	disco := &fake.FakeDiscovery{Fake: &clienttesting.Fake{}}
	handler := &MetricHandler{
		dCli:        setupFakeDynamicClient(t, nil),
		discoClient: disco,
		metric: v1alpha1.Metric{
			Spec: v1alpha1.MetricSpec{
				Target: v1alpha1.GroupVersionKindTarget{
					Group:   gvk.Group,
					Version: gvk.Version,
					Kind:    gvk.Kind,
				},
			},
		},
		gaugeMetric: newTestGauge(t),
	}

	result, err := handler.Monitor(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != v1alpha1.PhaseFailed {
		t.Fatalf("unexpected phase: wanted=%s, got=%s", v1alpha1.PhaseFailed, result.Phase)
	}
}

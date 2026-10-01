package orchestrator

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
)

func newManagedHandler(t *testing.T, gvk schema.GroupVersionKind, resources []string) *ManagedHandler {
	t.Helper()
	return &ManagedHandler{
		client:      setupFakeClient(t, []string{managedAndServedCRD(gvk)}),
		dCli:        setupFakeDynamicClient(t, resources),
		metric:      v1alpha1.ManagedMetric{},
		gaugeMetric: newTestGauge(t),
	}
}

func TestManagedHandlerMonitorActive(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "kubernetes.m.crossplane.io",
		Version: "v1alpha1",
		Kind:    "Object",
	}

	handler := newManagedHandler(t, gvk, []string{fakeResource(gvk), fakeResource(gvk)})

	result, err := handler.Monitor(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != v1alpha1.PhaseActive {
		t.Fatalf("unexpected phase: wanted=%s, got=%s", v1alpha1.PhaseActive, result.Phase)
	}
	obs, ok := result.Observation.(*v1alpha1.ManagedObservation)
	if !ok {
		t.Fatalf("unexpected observation type: %T", result.Observation)
	}
	if obs.Resources != "2" {
		t.Fatalf("unexpected resource count: wanted=2, got=%s", obs.Resources)
	}
}

func TestManagedHandlerMonitorRecordsDefaultDimensions(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "kubernetes.m.crossplane.io",
		Version: "v1alpha1",
		Kind:    "Object",
	}

	gauge := newTestGauge(t)
	recorded := make(map[string]int64)
	gauge.SetPrometheusFunc(func(dims map[string]string, value int64) {
		recorded[dims[KIND]+"|"+dims["ready"]+"|"+dims["synced"]] = value
	})

	handler := &ManagedHandler{
		client:      setupFakeClient(t, []string{managedAndServedCRD(gvk)}),
		dCli:        setupFakeDynamicClient(t, []string{fakeResource(gvk), fakeResource(gvk)}),
		metric:      v1alpha1.ManagedMetric{},
		gaugeMetric: gauge,
	}

	_, err := handler.Monitor(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// fakeResource has Ready=True, Synced=True; each emits value=1
	key := "Object|true|true"
	if recorded[key] != 1 {
		t.Fatalf("unexpected record for %q: wanted=1, got=%d (all=%v)", key, recorded[key], recorded)
	}
}

func TestManagedHandlerMonitorCustomDimensions(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "kubernetes.m.crossplane.io",
		Version: "v1alpha1",
		Kind:    "Object",
	}

	gauge := newTestGauge(t)
	recorded := make(map[string]int64)
	gauge.SetPrometheusFunc(func(dims map[string]string, value int64) {
		recorded[dims["syncedStatus"]] += value
	})

	handler := &ManagedHandler{
		client: setupFakeClient(t, []string{managedAndServedCRD(gvk)}),
		dCli:   setupFakeDynamicClient(t, []string{fakeResource(gvk), fakeResource(gvk)}),
		metric: v1alpha1.ManagedMetric{
			Spec: v1alpha1.ManagedMetricSpec{
				Dimensions: []v1alpha1.Projection{
					{Name: "syncedStatus", FieldPath: "status.conditions[?(@.type=='Synced')].status", Type: v1alpha1.TypePrimitive},
				},
			},
		},
		gaugeMetric: gauge,
	}

	result, err := handler.Monitor(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != v1alpha1.PhaseActive {
		t.Fatalf("unexpected phase: %s", result.Phase)
	}
	// fakeResource has Synced=True; 2 resources × value=1
	if recorded["True"] != 2 {
		t.Fatalf("unexpected custom dimension count: wanted True=2, got=%v", recorded)
	}
}

func TestManagedHandlerMonitorNoCRDs(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "kubernetes.m.crossplane.io",
		Version: "v1alpha1",
		Kind:    "Object",
	}

	handler := &ManagedHandler{
		client:      setupFakeClient(t, nil), // no CRDs
		dCli:        setupFakeDynamicClient(t, []string{fakeResource(gvk)}),
		metric:      v1alpha1.ManagedMetric{},
		gaugeMetric: newTestGauge(t),
	}

	result, err := handler.Monitor(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Phase != v1alpha1.PhaseActive {
		t.Fatalf("unexpected phase: %s", result.Phase)
	}
	obs, ok := result.Observation.(*v1alpha1.ManagedObservation)
	if !ok {
		t.Fatalf("unexpected observation type: %T", result.Observation)
	}
	if obs.Resources != "0" {
		t.Fatalf("expected 0 resources when no CRDs present, got=%s", obs.Resources)
	}
}

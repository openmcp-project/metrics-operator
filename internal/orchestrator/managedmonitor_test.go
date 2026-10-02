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
		recorded[dims["ready"]+"|"+dims["synced"]] = value
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
	// fakeResource has Ready=True, Synced=True; 2 resources in one bucket → count=2
	key := "True|True"
	if recorded[key] != 2 {
		t.Fatalf("unexpected record for %q: wanted=2, got=%d (all=%v)", key, recorded[key], recorded)
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

func TestManagedHandlerMonitorBaseDimensions(t *testing.T) {
	first := schema.GroupVersionKind{Group: "one.example.io", Version: "v1", Kind: "First"}
	second := schema.GroupVersionKind{Group: "two.example.io", Version: "v2", Kind: "Second"}
	resources := []string{fakeResource(first), fakeResource(first), fakeResource(second), fakeResource(second)}
	crds := []string{managedAndServedCRD(first), managedAndServedCRD(second)}

	tests := []struct {
		name       string
		dimensions []v1alpha1.Projection
	}{
		{name: "nil defaults"},
		{name: "explicitly empty", dimensions: []v1alpha1.Projection{}},
		{
			name: "custom and reserved projections",
			dimensions: []v1alpha1.Projection{
				{Name: "syncedStatus", FieldPath: "status.conditions[?(@.type=='Synced')].status", Type: v1alpha1.TypePrimitive},
				{Name: KIND, FieldPath: "metadata.name", Type: v1alpha1.TypePrimitive},
				{Name: GROUP, FieldPath: "metadata.name", Type: v1alpha1.TypePrimitive},
				{Name: VERSION, FieldPath: "metadata.name", Type: v1alpha1.TypePrimitive},
				{Name: CLUSTER, FieldPath: "metadata.name", Type: v1alpha1.TypePrimitive},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterName := "authoritative-cluster"
			recorded := make(map[string]int64)
			recordCount := 0
			gauge := newTestGauge(t)
			gauge.SetPrometheusFunc(func(dims map[string]string, value int64) {
				if dims[CLUSTER] != "authoritative-cluster" {
					t.Errorf("cluster dimension was not authoritative: %v", dims)
				}
				if dims[KIND] != first.Kind && dims[KIND] != second.Kind {
					t.Errorf("missing actual kind dimension: %v", dims)
				}
				if dims[GROUP] != first.Group && dims[GROUP] != second.Group {
					t.Errorf("missing actual group dimension: %v", dims)
				}
				if dims[VERSION] != first.Version && dims[VERSION] != second.Version {
					t.Errorf("missing actual version dimension: %v", dims)
				}
				if len(tt.dimensions) > 0 && dims["syncedStatus"] != "True" {
					t.Errorf("unexpected custom projection value: %v", dims)
				}
				key := dims[KIND] + "|" + dims[GROUP] + "|" + dims[VERSION]
				recorded[key] += value
				recordCount++
			})
			handler := &ManagedHandler{
				client:      setupFakeClient(t, crds),
				dCli:        setupFakeDynamicClient(t, resources),
				metric:      v1alpha1.ManagedMetric{Spec: v1alpha1.ManagedMetricSpec{Dimensions: tt.dimensions}},
				clusterName: &clusterName,
				gaugeMetric: gauge,
			}
			if _, err := handler.Monitor(context.Background()); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, gvk := range []schema.GroupVersionKind{first, second} {
				key := gvk.Kind + "|" + gvk.Group + "|" + gvk.Version
				if recorded[key] != 2 {
					t.Errorf("unexpected count for %s: wanted=2, got=%d (all=%v)", key, recorded[key], recorded)
				}
			}
			if recordCount != 2 {
				t.Errorf("reserved dimensions split resource counts across %d records", recordCount)
			}
			if len(recorded) != 2 {
				t.Errorf("resources were combined or split into unexpected series: %v", recorded)
			}
		})
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

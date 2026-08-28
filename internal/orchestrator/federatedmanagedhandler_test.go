package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	ktesting "k8s.io/client-go/testing"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"github.com/openmcp-project/metrics-operator/internal/clientoptl"
)

func TestNewFederatedManagedHandler(t *testing.T) {
	cluster := "test-cluster"
	metric := v1alpha1.FederatedManagedMetric{}
	metric.Spec.Name = "fed-managed"
	gauge := newTestGauge(t)

	handler, err := NewFederatedManagedHandler(metric, QueryConfig{
		Client:      setupFakeClient(t, nil),
		RestConfig:  rest.Config{Host: "https://example.invalid"},
		ClusterName: &cluster,
	}, gauge)
	if err != nil {
		t.Fatalf("NewFederatedManagedHandler failed: %v", err)
	}
	if handler.client == nil {
		t.Fatal("expected client")
	}
	if handler.dCli == nil {
		t.Fatal("expected dynamic client")
	}
	if handler.discoClient == nil {
		t.Fatal("expected discovery client")
	}
	if handler.gauge != gauge {
		t.Fatal("gauge was not preserved")
	}
	if handler.clusterName == nil || *handler.clusterName != cluster {
		t.Fatalf("unexpected cluster name: %v", handler.clusterName)
	}
	if handler.metric.Spec.Name != metric.Spec.Name {
		t.Fatalf("unexpected metric name: wanted=%q, got=%q", metric.Spec.Name, handler.metric.Spec.Name)
	}
}

func TestFederatedManagedMonitorRecordsAggregatedObservation(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "kubernetes.m.crossplane.io",
		Version: "v1alpha1",
		Kind:    "Object",
	}
	cluster := "test-cluster"

	handler := FederatedManagedHandler{
		client:      setupFakeClient(t, []string{federatedManagedCRD(gvk)}),
		dCli:        setupFakeDynamicClient(t, []string{fakeResource(gvk), fakeResource(gvk)}),
		metric:      v1alpha1.FederatedManagedMetric{},
		gauge:       newTestGauge(t),
		clusterName: &cluster,
	}

	result, err := handler.Monitor(context.Background())
	if err != nil {
		t.Fatalf("Monitor failed: %v", err)
	}
	if result.Phase != v1alpha1.PhaseActive {
		t.Fatalf("unexpected phase: wanted=%s, got=%s", v1alpha1.PhaseActive, result.Phase)
	}
	if result.Observation.GetValue() != "2" {
		t.Fatalf("unexpected observation value: wanted=2, got=%s", result.Observation.GetValue())
	}
}

func TestFederatedManagedMonitorReportsListErrors(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "kubernetes.m.crossplane.io",
		Version: "v1alpha1",
		Kind:    "Object",
	}
	dynamicClient := setupFakeDynamicClient(t, []string{fakeResource(gvk)})
	dynamicClient.PrependReactor("list", "*", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("boom")
	})

	handler := FederatedManagedHandler{
		client: setupFakeClient(t, []string{federatedManagedCRD(gvk)}),
		dCli:   dynamicClient,
		metric: v1alpha1.FederatedManagedMetric{},
		gauge:  newTestGauge(t),
	}

	result, err := handler.Monitor(context.Background())
	if err != nil {
		t.Fatalf("Monitor returned unexpected error: %v", err)
	}
	if result.Phase != v1alpha1.PhaseFailed {
		t.Fatalf("unexpected phase: wanted=%s, got=%s", v1alpha1.PhaseFailed, result.Phase)
	}
	if result.Error == nil {
		t.Fatal("expected result error")
	}
}

func TestFederatedManagedRecordManagedResourceCountsAggregates(t *testing.T) {
	gvk := schema.GroupVersionKind{
		Group:   "kubernetes.m.crossplane.io",
		Version: "v1alpha1",
		Kind:    "Object",
	}
	cluster := "test-cluster"
	gauge := newTestGauge(t)

	records := make(map[string]int64)
	gauge.SetPrometheusFunc(func(dims map[string]string, value int64) {
		if _, ok := dims["UUID"]; ok {
			t.Errorf("unexpected UUID dimension: %v", dims)
		}
		records[dims[CLUSTER]+"|"+dims[KIND]+"|"+dims[APIVERSION]+"|"+dims["Ready"]+"|"+dims["Synced"]] = value
	})

	handler := FederatedManagedHandler{
		client:      setupFakeClient(t, []string{federatedManagedCRD(gvk)}),
		dCli:        setupFakeDynamicClient(t, []string{fakeResource(gvk), fakeResource(gvk)}),
		metric:      v1alpha1.FederatedManagedMetric{},
		gauge:       gauge,
		clusterName: &cluster,
	}

	count, err := handler.recordManagedResourceCounts(context.Background())
	if err != nil {
		t.Fatalf("recordManagedResourceCounts failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("unexpected resource count: wanted=2, got=%d", count)
	}
	key := "test-cluster|Object|kubernetes.m.crossplane.io/v1alpha1|true|true"
	if records[key] != 2 {
		t.Fatalf("unexpected aggregated records: wanted %q=2, got %#v", key, records)
	}
	if len(records) != 1 {
		t.Fatalf("unexpected record count: wanted=1, got=%d (%#v)", len(records), records)
	}
}

func TestFederatedManagedRecordManagedResourceCountsUsesStorageVersion(t *testing.T) {
	storageGVK := schema.GroupVersionKind{
		Group:   "kubernetes.m.crossplane.io",
		Version: "v1",
		Kind:    "Object",
	}
	oldGVK := storageGVK
	oldGVK.Version = "v1beta1"

	handler := FederatedManagedHandler{
		client: setupFakeClient(t, []string{fmt.Sprintf(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: objects.kubernetes.m.crossplane.io
spec:
  group: kubernetes.m.crossplane.io
  names:
    categories:
    - crossplane
    - managed
    kind: Object
    listKind: ObjectList
    plural: objects
    singular: object
  scope: Cluster
  versions:
  - name: %s
    served: true
    storage: false
  - name: %s
    served: true
    storage: true
status:
  storedVersions:
  - %s
  - %s
`, oldGVK.Version, storageGVK.Version, oldGVK.Version, storageGVK.Version)}),
		dCli: setupFakeDynamicClient(t, []string{
			fakeResource(oldGVK),
			fakeResource(storageGVK),
		}),
		metric: v1alpha1.FederatedManagedMetric{},
		gauge:  newTestGauge(t),
	}

	count, err := handler.recordManagedResourceCounts(context.Background())
	if err != nil {
		t.Fatalf("recordManagedResourceCounts failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("unexpected resource count: wanted=1, got=%d", count)
	}
}

func TestFederatedManagedClusterDimensionDefaultsToEmpty(t *testing.T) {
	handler := FederatedManagedHandler{}
	if got := handler.clusterDimension(); got != "" {
		t.Fatalf("unexpected cluster dimension: wanted empty, got=%q", got)
	}
}

func TestFederatedManagedConditionStatus(t *testing.T) {
	tests := []struct {
		name          string
		resource      string
		conditionType string
		want          string
	}{
		{
			name:          "present condition",
			resource:      fakeResource(schema.GroupVersionKind{Group: "kubernetes.m.crossplane.io", Version: "v1alpha1", Kind: "Object"}),
			conditionType: "Ready",
			want:          "true",
		},
		{
			name: "missing conditions",
			resource: `apiVersion: kubernetes.m.crossplane.io/v1alpha1
kind: Object
metadata:
  name: missing
`,
			conditionType: "Ready",
			want:          "unknown",
		},
		{
			name:          "missing condition type",
			resource:      fakeResource(schema.GroupVersionKind{Group: "kubernetes.m.crossplane.io", Version: "v1alpha1", Kind: "Object"}),
			conditionType: "Missing",
			want:          "unknown",
		},
		{
			name: "non string status",
			resource: `apiVersion: kubernetes.m.crossplane.io/v1alpha1
kind: Object
metadata:
  name: bad-status
status:
  conditions:
  - type: Ready
    status: true
`,
			conditionType: "Ready",
			want:          "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := toUnstructured(t, tt.resource)
			if got := conditionStatus(&item, tt.conditionType); got != tt.want {
				t.Fatalf("unexpected condition status: wanted=%q, got=%q", tt.want, got)
			}
		})
	}
}

func newTestGauge(t *testing.T) *clientoptl.Metric {
	t.Helper()

	metricClient, err := clientoptl.NewMetricClient(context.Background(), nil)
	if err != nil {
		t.Fatalf("failed to create metric client: %v", err)
	}
	metricClient.SetMeter("test")

	gauge, err := metricClient.NewMetric("test_metric")
	if err != nil {
		t.Fatalf("failed to create gauge: %v", err)
	}

	return gauge
}

func federatedManagedCRD(gvk schema.GroupVersionKind) string {
	return managedAndServedCRD(gvk) + "    storage: true\n" + `status:
  storedVersions:
  - ` + gvk.Version + `
`
}

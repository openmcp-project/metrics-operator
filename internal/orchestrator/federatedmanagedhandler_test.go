package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	gvk := schema.GroupVersionKind{Group: "kubernetes.m.crossplane.io", Version: "v1alpha1", Kind: "Object"}
	cluster := "test-cluster"
	gauge := newTestGauge(t)
	records := make(map[string]int64)
	gauge.SetPrometheusFunc(func(dims map[string]string, value int64) error {
		key := dims[GROUP] + "|" + dims[VERSION] + "|" + dims[KIND] + "|" + dims[CLUSTER] + "|" + dims[APIVERSION] + "|" + dims["Ready"] + "|" + dims["Synced"]
		records[key] = value
		return nil
	})
	handler := FederatedManagedHandler{
		client: setupFakeClient(t, []string{federatedManagedCRD(gvk)}),
		dCli:   setupFakeDynamicClient(t, []string{fakeResource(gvk), fakeResource(gvk)}),
		metric: v1alpha1.FederatedManagedMetric{}, gauge: gauge, clusterName: &cluster,
	}
	count, err := handler.recordManagedResourceCounts(context.Background())
	if err != nil || count != 2 {
		t.Fatalf("recordManagedResourceCounts = (%d, %v), want (2, nil)", count, err)
	}
	key := "kubernetes.m.crossplane.io|v1alpha1|Object|test-cluster|kubernetes.m.crossplane.io/v1alpha1|True|True"
	if records[key] != 2 || len(records) != 1 {
		t.Fatalf("unexpected records: %#v", records)
	}
}

func TestFederatedManagedMissingConditionsUseUnknown(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "kubernetes.m.crossplane.io", Version: "v1alpha1", Kind: "Object"}
	resource := `apiVersion: kubernetes.m.crossplane.io/v1alpha1
kind: Object
metadata:
  name: no-conditions
`
	gauge := newTestGauge(t)
	var got map[string]string
	gauge.SetPrometheusFunc(func(dims map[string]string, _ int64) error { got = dims; return nil })
	handler := FederatedManagedHandler{
		client: setupFakeClient(t, []string{federatedManagedCRD(gvk)}),
		dCli:   setupFakeDynamicClient(t, []string{resource}),
		gauge:  gauge,
	}
	count, err := handler.recordManagedResourceCounts(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("recordManagedResourceCounts = (%d, %v), want (1, nil)", count, err)
	}
	if got["Ready"] != "unknown" || got["Synced"] != "unknown" {
		t.Fatalf("missing conditions were not represented as unknown: %#v", got)
	}
}

func TestFederatedManagedPaginationMergesBuckets(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "kubernetes.m.crossplane.io", Version: "v1alpha1", Kind: "Object"}
	dynamicClient := setupFakeDynamicClient(t, []string{fakeResource(gvk)})
	var listCalls int
	dynamicClient.PrependReactor("list", "objects", func(action ktesting.Action) (bool, runtime.Object, error) {
		listCalls++
		listAction := action.(ktesting.ListAction)
		if listAction.GetListRestrictions().Labels.String() != "" {
			t.Fatalf("unexpected list label selector")
		}
		if listCalls == 1 {
			page := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{toUnstructured(t, fakeResource(gvk))}}
			page.SetContinue("page-2")
			return true, page, nil
		}
		page := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{toUnstructured(t, fakeResource(gvk))}}
		return true, page, nil
	})
	var got int64
	gauge := newTestGauge(t)
	gauge.SetPrometheusFunc(func(dims map[string]string, value int64) error {
		got = value
		if dims[GROUP] != gvk.Group || dims[VERSION] != gvk.Version || dims[KIND] != gvk.Kind || dims[APIVERSION] != gvk.GroupVersion().String() {
			t.Errorf("unexpected base dimensions: %#v", dims)
		}
		return nil
	})
	handler := FederatedManagedHandler{client: setupFakeClient(t, []string{federatedManagedCRD(gvk)}), dCli: dynamicClient, gauge: gauge}
	count, err := handler.recordManagedResourceCounts(context.Background())
	if err != nil || count != 2 {
		t.Fatalf("recordManagedResourceCounts = (%d, %v), want (2, nil)", count, err)
	}
	if listCalls != 2 || got != 2 {
		t.Fatalf("pagination did not merge one bucket: calls=%d, value=%d", listCalls, got)
	}
}

func TestFederatedManagedListErrorRecordsNoPartialMetrics(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "kubernetes.m.crossplane.io", Version: "v1alpha1", Kind: "Object"}
	dynamicClient := setupFakeDynamicClient(t, []string{fakeResource(gvk)})
	var listCalls int
	dynamicClient.PrependReactor("list", "objects", func(action ktesting.Action) (bool, runtime.Object, error) {
		listCalls++
		if listCalls == 1 {
			page := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{toUnstructured(t, fakeResource(gvk))}}
			page.SetContinue("page-2")
			return true, page, nil
		}
		return true, nil, errors.New("page failed")
	})
	records := 0
	gauge := newTestGauge(t)
	gauge.SetPrometheusFunc(func(map[string]string, int64) error { records++; return nil })
	handler := FederatedManagedHandler{client: setupFakeClient(t, []string{federatedManagedCRD(gvk)}), dCli: dynamicClient, gauge: gauge}
	count, err := handler.recordManagedResourceCounts(context.Background())
	if err == nil || count != 0 {
		t.Fatalf("recordManagedResourceCounts = (%d, %v), want (0, error)", count, err)
	}
	if records != 0 {
		t.Fatalf("recorded %d metrics before listing completed", records)
	}

}
func TestFederatedManagedRecordManagedResourceCountsPropagatesRecordErrors(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "kubernetes.m.crossplane.io", Version: "v1alpha1", Kind: "Object"}
	failed := errors.New("Prometheus registration failed")
	gauge := newTestGauge(t)
	gauge.SetPrometheusFunc(func(map[string]string, int64) error { return failed })
	handler := FederatedManagedHandler{
		client: setupFakeClient(t, []string{federatedManagedCRD(gvk)}),
		dCli:   setupFakeDynamicClient(t, []string{fakeResource(gvk)}),
		metric: v1alpha1.FederatedManagedMetric{},
		gauge:  gauge,
	}
	_, err := handler.recordManagedResourceCounts(context.Background())
	if !errors.Is(err, failed) {
		t.Fatalf("recordManagedResourceCounts error = %v, want %v", err, failed)
	}
}

func TestFederatedManagedRecordManagedResourceCountsUsesStorageVersion(t *testing.T) {
	storageGVK := schema.GroupVersionKind{Group: "kubernetes.m.crossplane.io", Version: "v1", Kind: "Object"}
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
    categories: [crossplane, managed]
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
`, oldGVK.Version, storageGVK.Version)}),
		dCli:  setupFakeDynamicClient(t, []string{fakeResource(oldGVK), fakeResource(storageGVK)}),
		gauge: newTestGauge(t),
	}
	count, err := handler.recordManagedResourceCounts(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("recordManagedResourceCounts = (%d, %v), want (1, nil)", count, err)
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

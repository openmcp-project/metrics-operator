package orchestrator

import (
	"testing"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"github.com/openmcp-project/metrics-operator/internal/clientoptl"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestGroupProjectionResultsByGVKIncludesEmptyProjections(t *testing.T) {
	gvks := []schema.GroupVersionKind{
		{Group: "one.io", Version: "v1", Kind: "One"},
		{Group: "two.io", Version: "v2", Kind: "Two"},
	}
	list := &unstructured.UnstructuredList{Items: make([]unstructured.Unstructured, len(gvks))}
	for i, gvk := range gvks {
		list.Items[i].SetGroupVersionKind(gvk)
	}
	groups := groupProjectionResultsByGVK(list, nil)
	if len(groups) != len(gvks) {
		t.Fatalf("expected one base group per GVK, got %d", len(groups))
	}
	for _, group := range groups {
		if len(group) != 1 || len(group[0]) != 0 {
			t.Fatalf("expected empty projection fields for one object, got %#v", group)
		}
	}
}

func TestMetricBaseDimensionsAreNotOverriddenByCustomProjection(t *testing.T) {
	gvk := schema.GroupVersionKind{Group: "actual.io", Version: "v1", Kind: "Actual"}
	list := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{}}}
	list.Items[0].SetGroupVersionKind(gvk)
	groups := groupProjectionResultsByGVK(list, []v1alpha1.Projection{{Name: RESOURCE, FieldPath: "metadata.name"}})
	for _, group := range groups {
		if len(group) != 1 || group[0][0].gvk != gvk {
			t.Fatalf("custom base-name projection lost actual identity: %#v", group)
		}
	}
	gauge := newTestGauge(t)
	var dims map[string]string
	gauge.SetPrometheusFunc(func(got map[string]string, _ int64) { dims = got })
	dp := clientoptl.NewDataPoint()
	handler := &MetricHandler{clusterName: nil}
	handler.setDataPointBaseDimensionsFor(dp, gvk)
	dp.AddDimension(RESOURCE, "not-actual")
	if err := gauge.RecordMetrics(t.Context(), dp); err != nil {
		t.Fatalf("record metric: %v", err)
	}
	if dims[RESOURCE] != gvk.Kind || dims[GROUP] != gvk.Group || dims[VERSION] != gvk.Version {
		t.Fatalf("base identity was overridden: %v", dims)
	}
}

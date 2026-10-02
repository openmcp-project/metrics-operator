package orchestrator

import (
	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"testing"
)

func TestMetricCountsKeepActualIdentity(t *testing.T) {
	for _, custom := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "custom"}[custom], func(t *testing.T) {
			first := schema.GroupVersionKind{Group: "one.io", Version: "v1", Kind: "One"}
			second := schema.GroupVersionKind{Group: "two.io", Version: "v2", Kind: "Two"}
			list := &unstructured.UnstructuredList{}
			for i, gvk := range []schema.GroupVersionKind{first, first, second} {
				obj := unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"annotations": map[string]any{"cluster": []string{"fake-a", "fake-b", "fake-c"}[i], "team": "platform"}}}}
				obj.SetGroupVersionKind(gvk)
				list.Items = append(list.Items, obj)
			}
			var projections []v1alpha1.Projection
			if custom {
				projections = []v1alpha1.Projection{{Name: CLUSTER, FieldPath: "metadata.annotations.cluster", Type: v1alpha1.TypePrimitive}, {Name: "team", FieldPath: "metadata.annotations.team", Type: v1alpha1.TypePrimitive}}
			}
			cluster := "actual"
			gauge := newTestGauge(t)
			counts := map[string]int64{}
			gauge.SetPrometheusFunc(func(dims map[string]string, value int64) error {
				if dims[CLUSTER] != cluster {
					t.Errorf("wrong cluster: %v", dims)
				}
				if custom && dims["team"] != "platform" {
					t.Errorf("missing custom dimension: %v", dims)
				}
				counts[dims[GROUP]+"/"+dims[VERSION]+"/"+dims[KIND]] = value
				return nil
			})
			h := MetricHandler{metric: v1alpha1.Metric{Spec: v1alpha1.MetricSpec{Projections: projections}}, clusterName: &cluster, gaugeMetric: gauge}
			result, err := h.projectionsMonitor(t.Context(), list, targetLookupResult{})
			if err != nil || result.Error != nil {
				t.Fatalf("monitor: %v / %v", err, result.Error)
			}
			if len(counts) != 2 || counts["one.io/v1/One"] != 2 || counts["two.io/v2/Two"] != 1 {
				t.Fatalf("wrong identity counts: %v", counts)
			}
		})
	}
}

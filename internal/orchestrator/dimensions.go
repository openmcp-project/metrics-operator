package orchestrator

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"github.com/openmcp-project/metrics-operator/internal/clientoptl"
)

func addClusterDimension(dataPoint *clientoptl.DataPoint, clusterName *string) {
	if clusterName != nil && *clusterName != "" {
		dataPoint.AddDimension(CLUSTER, *clusterName)
	}
}

func addTargetGVKDimensions(dataPoint *clientoptl.DataPoint, target *v1alpha1.GroupVersionKindTarget) {
	if target == nil {
		return
	}
	addGVKDimensions(dataPoint, TARGET_KIND, TARGET_GROUP, TARGET_VERSION, TARGET_APIVERSION, target.GVK())
}

func addResourceGVKDimensions(dataPoint *clientoptl.DataPoint, gvk schema.GroupVersionKind) {
	addGVKDimensions(dataPoint, CR_KIND, CR_GROUP, CR_VERSION, CR_APIVERSION, gvk)
}

func addGVKDimensions(dataPoint *clientoptl.DataPoint, kindKey, groupKey, versionKey, apiVersionKey string, gvk schema.GroupVersionKind) {
	dataPoint.AddDimension(kindKey, dimensionValue(gvk.Kind))
	dataPoint.AddDimension(groupKey, dimensionValue(gvk.Group))
	dataPoint.AddDimension(versionKey, dimensionValue(gvk.Version))
	dataPoint.AddDimension(apiVersionKey, dimensionValue(gvk.GroupVersion().String()))
}

func resourceGVKProjectedFields(obj unstructured.Unstructured) []projectedField {
	gvk := obj.GroupVersionKind()
	return []projectedField{
		{name: CR_KIND, value: dimensionValue(gvk.Kind), found: gvk.Kind != ""},
		{name: CR_GROUP, value: dimensionValue(gvk.Group), found: true},
		{name: CR_VERSION, value: dimensionValue(gvk.Version), found: gvk.Version != ""},
		{name: CR_APIVERSION, value: dimensionValue(gvk.GroupVersion().String()), found: gvk.Version != ""},
	}
}

func dimensionValue(value string) string {
	if value == "" {
		return "n/a"
	}
	return value
}

func gvkFromAPIVersionAndKind(apiVersion, kind string) schema.GroupVersionKind {
	return schema.FromAPIVersionAndKind(apiVersion, kind)
}

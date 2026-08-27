package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"github.com/openmcp-project/metrics-operator/internal/clientoptl"
)

func TestExplicitGVKDimensions(t *testing.T) {
	target := &v1alpha1.GroupVersionKindTarget{Kind: "Pod", Version: "v1"}
	dataPoint := clientoptl.NewDataPoint()

	addTargetGVKDimensions(dataPoint, target)
	addResourceGVKDimensions(dataPoint, schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})

	require.Equal(t, "Pod", dataPoint.Dimensions[TARGET_KIND])
	require.Equal(t, "n/a", dataPoint.Dimensions[TARGET_GROUP])
	require.Equal(t, "v1", dataPoint.Dimensions[TARGET_VERSION])
	require.Equal(t, "v1", dataPoint.Dimensions[TARGET_APIVERSION])
	require.Equal(t, "Deployment", dataPoint.Dimensions[CR_KIND])
	require.Equal(t, "apps", dataPoint.Dimensions[CR_GROUP])
	require.Equal(t, "v1", dataPoint.Dimensions[CR_VERSION])
	require.Equal(t, "apps/v1", dataPoint.Dimensions[CR_APIVERSION])
}

func TestExtractProjectionGroupsWithResourceGVK(t *testing.T) {
	pod := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "pod",
			"namespace": "default",
		},
	}}
	deployment := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      "deployment",
			"namespace": "default",
		},
	}}
	list := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{pod, deployment}}

	groups := extractProjectionGroupsFromWithResourceGVK(list, nil, true)

	require.Len(t, groups, 2)
	require.Contains(t, groups, "cr_kind: Pod,cr_group: n/a,cr_version: v1,cr_api_version: v1")
	require.Contains(t, groups, "cr_kind: Deployment,cr_group: apps,cr_version: v1,cr_api_version: apps/v1")
}

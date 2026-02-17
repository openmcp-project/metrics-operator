package orchestrator_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"github.com/openmcp-project/metrics-operator/internal/orchestrator"
)

func TestExtractProjectionGroupsFrom(t *testing.T) {
	tests := []struct {
		name                     string
		projections              []v1alpha1.Projection
		objects                  []unstructured.Unstructured
		expectedProjectionGroups orchestrator.ProjectionGroups
	}{
		{
			name: "Test with single projection",
			projections: []v1alpha1.Projection{
				{
					Name:      "namespace",
					FieldPath: "metadata.namespace",
					Type:      string(v1alpha1.TypePrimitive),
				},
			},
			objects: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"name":      "test",
							"namespace": "default",
						},
					},
				},
			},
			expectedProjectionGroups: orchestrator.ProjectionGroups{
				"namespace: default": {
					{
						{
							Name:  "namespace",
							Value: "default",
							Found: true,
							Error: nil,
						},
					},
				},
			},
		},

		{
			name: "Test with multiple projections",
			projections: []v1alpha1.Projection{
				{
					Name:      "namespace",
					FieldPath: "metadata.namespace",
					Type:      string(v1alpha1.TypePrimitive),
				},
				{
					Name:      "name",
					FieldPath: "metadata.name",
					Type:      string(v1alpha1.TypePrimitive),
				},
			},
			objects: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"name":      "test1",
							"namespace": "default",
						},
					},
				},
				{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"name":      "test2",
							"namespace": "default",
						},
					},
				},
			},
			expectedProjectionGroups: orchestrator.ProjectionGroups{
				"namespace: default,name: test1": {
					{
						{
							Name:  "namespace",
							Value: "default",
							Found: true,
							Error: nil,
						},
						{
							Name:  "name",
							Value: "test1",
							Found: true,
							Error: nil,
						},
					},
				},
				"namespace: default,name: test2": {
					{
						{
							Name:  "namespace",
							Value: "default",
							Found: true,
							Error: nil,
						},
						{
							Name:  "name",
							Value: "test2",
							Found: true,
							Error: nil,
						},
					},
				},
			},
		},
		{
			name: "Test with projections matching multiple objects",
			projections: []v1alpha1.Projection{
				{
					Name:      "namespace",
					FieldPath: "metadata.namespace",
					Type:      string(v1alpha1.TypePrimitive),
				},
				{
					Name:      "label",
					FieldPath: "metadata.labels.app",
					Type:      string(v1alpha1.TypePrimitive),
				},
			},
			objects: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"name":      "test1",
							"namespace": "default",
							"labels": map[string]interface{}{
								"app": "myapp",
							},
						},
					},
				},
				{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"name":      "test2",
							"namespace": "default",
							"labels": map[string]interface{}{
								"app": "myapp",
							},
						},
					},
				},
			},
			expectedProjectionGroups: orchestrator.ProjectionGroups{
				"namespace: default,label: myapp": {
					{
						{
							Name:  "namespace",
							Value: "default",
							Found: true,
							Error: nil,
						},
						{
							Name:  "label",
							Value: "myapp",
							Found: true,
							Error: nil,
						},
					},
					{
						{
							Name:  "namespace",
							Value: "default",
							Found: true,
							Error: nil,
						},
						{
							Name:  "label",
							Value: "myapp",
							Found: true,
							Error: nil,
						},
					},
				},
			},
		},
		{
			name: "Test with missing projection value",
			projections: []v1alpha1.Projection{
				{
					Name:      "namespace",
					FieldPath: "metadata.namespace",
					Type:      string(v1alpha1.TypePrimitive),
				},
				{
					Name:      "label",
					FieldPath: "metadata.labels.app",
					Type:      string(v1alpha1.TypePrimitive),
				},
			},
			objects: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"name":      "test1",
							"namespace": "default",
						},
					},
				},
				{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"name":      "test2",
							"namespace": "default",
							"labels": map[string]interface{}{
								"app": "myapp",
							},
						},
					},
				},
			},
			expectedProjectionGroups: orchestrator.ProjectionGroups{
				"namespace: default,label: ": {
					{
						{
							Name:  "namespace",
							Value: "default",
							Found: true,
							Error: nil,
						},
						{
							Name:  "label",
							Value: "",
							Found: false,
							Error: nil,
						},
					},
				},
				"namespace: default,label: myapp": {
					{
						{
							Name:  "namespace",
							Value: "default",
							Found: true,
							Error: nil,
						},
						{
							Name:  "label",
							Value: "myapp",
							Found: true,
							Error: nil,
						},
					},
				},
			},
		},
		{
			name: "Test with invalid field path",
			projections: []v1alpha1.Projection{
				{
					Name:      "invalid",
					FieldPath: "metadata.nonexistent",
					Type:      string(v1alpha1.TypePrimitive),
				},
			},
			objects: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"name":      "test",
							"namespace": "default",
						},
					},
				},
			},
			expectedProjectionGroups: orchestrator.ProjectionGroups{
				"invalid: ": {
					{
						{
							Name:  "invalid",
							Value: "",
							Found: false,
							Error: nil,
						},
					},
				},
			},
		},
		{
			name:        "Test with empty projections",
			projections: []v1alpha1.Projection{},
			objects: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"metadata": map[string]interface{}{
							"name":      "test",
							"namespace": "default",
						},
					},
				},
			},
			expectedProjectionGroups: orchestrator.ProjectionGroups{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objectList := &unstructured.UnstructuredList{
				Items: tt.objects,
			}

			projectionGroups := orchestrator.ExtractProjectionGroupsFrom(objectList, tt.projections)
			require.NotNil(t, projectionGroups)
			require.Len(t, projectionGroups, len(tt.expectedProjectionGroups))
			for key, expectedGroup := range tt.expectedProjectionGroups {
				group, exists := projectionGroups[key]
				require.True(t, exists, "expected projection group key '%s' not found", key)
				require.Equal(t, expectedGroup, group, "projection group for key '%s' does not match expected", key)
			}
		})
	}
}

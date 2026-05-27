package orchestrator

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
)

const subaccountCR = `
apiVersion: nop.crossplane.io/v1alpha1
kind: NopResource
metadata:
  annotations:
    crossplane.io/external-name: ext-example
    crossplane.io/external-create-succeeded: "2025-11-19T09:26:05Z"
  name: example
  labels:
    app: myapp
    env: prod
    team: platform
spec:
  deletionPolicy: Delete
  forProvider:
    tags:
      - name: tag1
        value: value1
    emptyList: []
    config:
      nested: value
status:
  conditions:
  - lastTransitionTime: "2025-09-12T15:57:41Z"
    observedGeneration: 1
    reason: ReconcileSuccess
    status: "True"
    type: Synced
  - lastTransitionTime: "2025-09-09T14:33:38Z"
    reason: Available
    status: "True"
    type: Ready
  emptyConditions: []
  atProvider:
    id: "12345"
    nullValue: null
    boolValue: true
    intValue: 42
    floatValue: 3.14
`

// Helper function to run tests
func runTests(t *testing.T, tests []struct {
	name         string
	resourceYaml string
	path         string
	valueType    v1alpha1.DimensionType
	defaultValue *v1alpha1.ProjectionDefaultValue
	wantValue    string
	wantFound    bool
	wantError    bool
}) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := toUnstructured(t, tt.resourceYaml)
			value, ok, err := nestedFieldValue(obj, tt.path, tt.valueType, tt.defaultValue)

			if (err != nil) != tt.wantError {
				t.Errorf("unexpected error: got %v, wantErr %v", err, tt.wantError)
			}
			if ok != tt.wantFound {
				t.Errorf("unexpected ok result: got %v, want %v", ok, tt.wantFound)
			}
			// For complex types (like maps), parse both JSON strings and compare the objects
			if tt.valueType == v1alpha1.TypeMap && value != "" && tt.wantValue != "" {
				var gotObj, wantObj map[string]interface{}

				// Parse the actual value
				if err := json.Unmarshal([]byte(value), &gotObj); err != nil {
					t.Fatalf("failed to parse got value as JSON: %v", err)
				}

				// Parse the expected value
				if err := json.Unmarshal([]byte(tt.wantValue), &wantObj); err != nil {
					t.Fatalf("failed to parse want value as JSON: %v", err)
				}

				// Compare the objects
				if !reflect.DeepEqual(gotObj, wantObj) {
					t.Errorf("unexpected value:\ngot:  %v\nwant: %v", value, tt.wantValue)
				}
				return
			} else {
				if value != tt.wantValue {
					t.Errorf("unexpected value: got %v, want %v", value, tt.wantValue)
				}
			}
		})
	}
}

func toUnstructured(t *testing.T, resourceYaml string) unstructured.Unstructured {
	t.Helper()
	var object map[string]interface{}
	if err := yaml.Unmarshal([]byte(resourceYaml), &object); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}
	return unstructured.Unstructured{Object: object}
}

// tests the TypePrimitive functionality
func TestNestedFieldValue_primitive(t *testing.T) {
	tests := []struct {
		name         string
		resourceYaml string
		path         string
		valueType    v1alpha1.DimensionType
		defaultValue *v1alpha1.ProjectionDefaultValue
		wantValue    string
		wantFound    bool
		wantError    bool
	}{
		{
			name:         "top level value retrieval",
			resourceYaml: subaccountCR,
			path:         "kind",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "NopResource",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with name selector",
			resourceYaml: subaccountCR,
			path:         "spec.deletionPolicy",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "Delete",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with escaped name selector",
			resourceYaml: subaccountCR,
			path:         "metadata.annotations.crossplane\\.io/external-name",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "ext-example",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with index selector",
			resourceYaml: subaccountCR,
			path:         "status.conditions[1].status",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "True",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with filter selector",
			resourceYaml: subaccountCR,
			path:         "status.conditions[?(@.type=='Ready')].status",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "True",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with array slice selector",
			resourceYaml: subaccountCR,
			path:         "status.conditions[0:1].status",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "True",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with wildcard selector; collection results are not supported",
			resourceYaml: subaccountCR,
			path:         "status.conditions[*].status",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "non-existent value",
			resourceYaml: subaccountCR,
			path:         "metadata.labels.nonexistent",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    false,
			wantError:    false,
		},
		{
			name:         "nested non-string value retrieval with default print format",
			resourceYaml: subaccountCR,
			path:         "status.conditions[0].observedGeneration",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "1",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "retrieval of collection types is not supported for primitive",
			resourceYaml: subaccountCR,
			path:         "status.conditions[0]",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "invalid array index returns an error",
			resourceYaml: subaccountCR,
			path:         "status.conditions[abc].status",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    false,
			wantError:    true,
		},
		{
			name:         "invalid path syntax returns an error",
			resourceYaml: subaccountCR,
			path:         "$.[status.conditions[0].status]",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    false,
			wantError:    true,
		},
		{
			name:         "TypePrimitive handles boolean value",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.boolValue",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "true",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypePrimitive handles integer value",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.intValue",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "42",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypePrimitive handles float value",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.floatValue",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "3.14",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypePrimitive on map field should error",
			resourceYaml: subaccountCR,
			path:         "metadata.labels",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypePrimitive on array field should error",
			resourceYaml: subaccountCR,
			path:         "status.conditions",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypePrimitive on null value",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.nullValue",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "null",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "root path withv1alpha1.TypePrimitive should error",
			resourceYaml: subaccountCR,
			path:         ".",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "filter returning no results",
			resourceYaml: subaccountCR,
			path:         "status.conditions[?(@.type=='NonExistent')]",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    false,
			wantError:    false,
		},
		{
			name:         "TypePrimitive on empty string field",
			resourceYaml: `{"metadata":{"name":""}}`,
			path:         "metadata.name",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "Deeply nested non-existent path",
			resourceYaml: subaccountCR,
			path:         "spec.missing.deeply.nested.path",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    false,
			wantError:    false,
		},
		{
			name:         "TypePrimitive with default value on non-existent field",
			resourceYaml: subaccountCR,
			path:         "metadata.nonexistent",
			valueType:    v1alpha1.TypePrimitive,
			defaultValue: v1alpha1.NewProjectionDefaultValue("default"),
			wantValue:    "default",
			wantFound:    true,
			wantError:    false,
		},
	}
	runTests(t, tests)
}

// tests the TypeMap functionality
func TestNestedFieldValue_map(t *testing.T) {
	tests := []struct {
		name         string
		resourceYaml string
		path         string
		valueType    v1alpha1.DimensionType
		defaultValue *v1alpha1.ProjectionDefaultValue
		wantValue    string
		wantFound    bool
		wantError    bool
	}{
		{
			name:         "TypeMap on map field should succeed",
			resourceYaml: subaccountCR,
			path:         "metadata.labels",
			valueType:    v1alpha1.TypeMap,
			wantValue:    `{"app":"myapp","env":"prod","team":"platform"}`,
			defaultValue: nil,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeMap on nested map should succeed",
			resourceYaml: subaccountCR,
			path:         "spec.forProvider.config",
			valueType:    v1alpha1.TypeMap,
			wantValue:    `{"nested":"value"}`,
			defaultValue: nil,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeMap on empty map should succeed",
			resourceYaml: `{"metadata":{"labels":{}}}`,
			path:         "metadata.labels",
			valueType:    v1alpha1.TypeMap,
			defaultValue: nil,
			wantValue:    `{}`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeMap on entire status.atProvider",
			resourceYaml: subaccountCR,
			path:         "status.atProvider",
			valueType:    v1alpha1.TypeMap,
			defaultValue: nil,
			wantValue:    `{"id": "12345","nullValue": null,"boolValue": true,"intValue": 42,"floatValue": 3.14}`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "root path withv1alpha1.TypeMap should succeed",
			resourceYaml: subaccountCR,
			path:         ".",
			valueType:    v1alpha1.TypeMap,
			defaultValue: nil,
			wantValue:    `{"apiVersion":"nop.crossplane.io/v1alpha1","kind":"NopResource","metadata":{"annotations":{"crossplane.io/external-name":"ext-example","crossplane.io/external-create-succeeded":"2025-11-19T09:26:05Z"},"name":"example","labels":{"app":"myapp","env":"prod","team":"platform"}},"spec":{"deletionPolicy":"Delete","forProvider":{"tags":[{"name":"tag1","value":"value1"}],"emptyList":[],"config":{"nested":"value"}}},"status":{"conditions":[{"lastTransitionTime":"2025-09-12T15:57:41Z","observedGeneration":1,"reason":"ReconcileSuccess","status":"True","type":"Synced"},{"lastTransitionTime":"2025-09-09T14:33:38Z","reason":"Available","status":"True","type":"Ready"}],"emptyConditions":[],"atProvider":{"id":"12345","nullValue":null,"boolValue":true,"intValue":42,"floatValue":3.14}}}`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeMap on primitive field should error",
			resourceYaml: subaccountCR,
			path:         "kind",
			valueType:    v1alpha1.TypeMap,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypeMap on array field should error",
			resourceYaml: subaccountCR,
			path:         "status.conditions",
			valueType:    v1alpha1.TypeMap,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypeMap with wildcard should error on multiple results",
			resourceYaml: subaccountCR,
			path:         "status.conditions[*]",
			valueType:    v1alpha1.TypeMap,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypeMap with slice notation should error on multiple results",
			resourceYaml: subaccountCR,
			path:         "status.conditions[0:2]",
			valueType:    v1alpha1.TypeMap,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypeMap on null value should error",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.nullValue",
			valueType:    v1alpha1.TypeMap,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypeMap on non-existent field",
			resourceYaml: subaccountCR,
			path:         "metadata.nonexistent",
			valueType:    v1alpha1.TypeMap,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    false,
			wantError:    false,
		},
		{
			name:         "TypeMap on annotations",
			resourceYaml: subaccountCR,
			path:         "metadata.annotations",
			valueType:    v1alpha1.TypeMap,
			defaultValue: nil,
			wantValue:    `{"crossplane.io/external-name":"ext-example","crossplane.io/external-create-succeeded":"2025-11-19T09:26:05Z"}`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeMap with default value on non-existent field",
			resourceYaml: subaccountCR,
			path:         "metadata.nonexistent",
			valueType:    v1alpha1.TypeMap,
			defaultValue: v1alpha1.NewProjectionDefaultValue(map[string]string{"defaultKey": "defaultValue"}),
			wantValue:    `{"defaultKey":"defaultValue"}`,
			wantFound:    true,
			wantError:    false,
		},
	}

	runTests(t, tests)
}

// tests the TypeSlice functionality
func TestNestedFieldValue_slice(t *testing.T) {
	tests := []struct {
		name         string
		resourceYaml string
		path         string
		valueType    v1alpha1.DimensionType
		defaultValue *v1alpha1.ProjectionDefaultValue
		wantValue    string
		wantFound    bool
		wantError    bool
	}{
		{
			name:         "TypeSlice on array field should succeed",
			resourceYaml: subaccountCR,
			path:         "status.conditions",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    `[{"lastTransitionTime":"2025-09-12T15:57:41Z","observedGeneration":1,"reason":"ReconcileSuccess","status":"True","type":"Synced"},{"lastTransitionTime":"2025-09-09T14:33:38Z","reason":"Available","status":"True","type":"Ready"}]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice on empty array should succeed",
			resourceYaml: subaccountCR,
			path:         "status.emptyConditions",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    `[]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice wraps single primitive from filter",
			resourceYaml: subaccountCR,
			path:         "status.conditions[?(@.type=='Ready')].status",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    `["True"]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice wraps single map from array index",
			resourceYaml: subaccountCR,
			path:         "status.conditions[0]",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    `[{"lastTransitionTime":"2025-09-12T15:57:41Z","observedGeneration":1,"reason":"ReconcileSuccess","status":"True","type":"Synced"}]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice on direct primitive path wraps in array",
			resourceYaml: subaccountCR,
			path:         "kind",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    `["NopResource"]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice with array slice returns multiple items",
			resourceYaml: subaccountCR,
			path:         "status.conditions[0:2].type",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    `["Synced","Ready"]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice with wildcard returns all items",
			resourceYaml: subaccountCR,
			path:         "status.conditions[*].type",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    `["Synced","Ready"]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice wraps single integer",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.intValue",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    `[42]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice wraps single boolean",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.boolValue",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    `[true]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice wraps single float",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.floatValue",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    `[3.14]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice on map field wraps map in array",
			resourceYaml: subaccountCR,
			path:         "metadata.labels",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    `[{"app":"myapp","env":"prod","team":"platform"}]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "root path withv1alpha1.TypeSlice should error",
			resourceYaml: subaccountCR,
			path:         ".",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypeSlice on non-existent field",
			resourceYaml: subaccountCR,
			path:         "status.nonexistent",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    "",
			wantFound:    false,
			wantError:    false,
		},
		{
			name:         "TypeSlice wraps null value",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.nullValue",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    `[null]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice on filter returning multiple primitives",
			resourceYaml: subaccountCR,
			path:         "status.conditions[*].status",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    `["True","True"]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice on a field that is a slice of primitive values",
			resourceYaml: `{"metadata":{"finalizers":["f1",12]}}`,
			path:         "metadata.finalizers",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: nil,
			wantValue:    `["f1",12]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice with default value on non-existent field",
			resourceYaml: subaccountCR,
			path:         "metadata.nonexistent",
			valueType:    v1alpha1.TypeSlice,
			defaultValue: v1alpha1.NewProjectionDefaultValue([]string{"defaultValue1", "defaultValue2"}),
			wantValue:    `["defaultValue1","defaultValue2"]`,
			wantFound:    true,
			wantError:    false,
		},
	}

	runTests(t, tests)
}

func TestExtractProjectionGroupsFrom(t *testing.T) {
	tests := []struct {
		name                     string
		projections              []v1alpha1.Projection
		objects                  []unstructured.Unstructured
		expectedProjectionGroups projectionGroups
	}{
		{
			name: "Test with single projection",
			projections: []v1alpha1.Projection{
				{
					Name:      "namespace",
					FieldPath: "metadata.namespace",
					Type:      v1alpha1.TypePrimitive,
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
			expectedProjectionGroups: projectionGroups{
				"namespace: default": {
					{
						{
							name:  "namespace",
							value: "default",
							found: true,
							error: nil,
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
					Type:      v1alpha1.TypePrimitive,
				},
				{
					Name:      "name",
					FieldPath: "metadata.name",
					Type:      v1alpha1.TypePrimitive,
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
			expectedProjectionGroups: projectionGroups{
				"namespace: default,name: test1": {
					{
						{
							name:  "namespace",
							value: "default",
							found: true,
							error: nil,
						},
						{
							name:  "name",
							value: "test1",
							found: true,
							error: nil,
						},
					},
				},
				"namespace: default,name: test2": {
					{
						{
							name:  "namespace",
							value: "default",
							found: true,
							error: nil,
						},
						{
							name:  "name",
							value: "test2",
							found: true,
							error: nil,
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
					Type:      v1alpha1.TypePrimitive,
				},
				{
					Name:      "label",
					FieldPath: "metadata.labels.app",
					Type:      v1alpha1.TypePrimitive,
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
			expectedProjectionGroups: projectionGroups{
				"namespace: default,label: myapp": {
					{
						{
							name:  "namespace",
							value: "default",
							found: true,
							error: nil,
						},
						{
							name:  "label",
							value: "myapp",
							found: true,
							error: nil,
						},
					},
					{
						{
							name:  "namespace",
							value: "default",
							found: true,
							error: nil,
						},
						{
							name:  "label",
							value: "myapp",
							found: true,
							error: nil,
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
					Type:      v1alpha1.TypePrimitive,
				},
				{
					Name:      "label",
					FieldPath: "metadata.labels.app",
					Type:      v1alpha1.TypePrimitive,
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
			expectedProjectionGroups: projectionGroups{
				"namespace: default,label: ": {
					{
						{
							name:  "namespace",
							value: "default",
							found: true,
							error: nil,
						},
						{
							name:  "label",
							value: "",
							found: false,
							error: nil,
						},
					},
				},
				"namespace: default,label: myapp": {
					{
						{
							name:  "namespace",
							value: "default",
							found: true,
							error: nil,
						},
						{
							name:  "label",
							value: "myapp",
							found: true,
							error: nil,
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
					Type:      v1alpha1.TypePrimitive,
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
			expectedProjectionGroups: projectionGroups{
				"invalid: ": {
					{
						{
							name:  "invalid",
							value: "",
							found: false,
							error: nil,
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
			expectedProjectionGroups: projectionGroups{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objectList := &unstructured.UnstructuredList{
				Items: tt.objects,
			}

			projectionGroups := extractProjectionGroupsFrom(objectList, tt.projections)
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

func TestNestedFieldValue_integer(t *testing.T) {
	runTests(t, []struct {
		name         string
		resourceYaml string
		path         string
		valueType    v1alpha1.DimensionType
		defaultValue *v1alpha1.ProjectionDefaultValue
		wantValue    string
		wantFound    bool
		wantError    bool
	}{
		{
			name:         "integer field",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.intValue",
			valueType:    v1alpha1.TypeInteger,
			wantValue:    "42",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "float with no fractional part is accepted",
			resourceYaml: `{"status":{"atProvider":{"val":100.0}}}`,
			path:         "status.atProvider.val",
			valueType:    v1alpha1.TypeInteger,
			wantValue:    "100",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "float with fractional part returns error",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.floatValue",
			valueType:    v1alpha1.TypeInteger,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "string field returns error",
			resourceYaml: subaccountCR,
			path:         "metadata.name",
			valueType:    v1alpha1.TypeInteger,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
	})
}

func TestNestedFieldValue_timestamp(t *testing.T) {
	runTests(t, []struct {
		name         string
		resourceYaml string
		path         string
		valueType    v1alpha1.DimensionType
		defaultValue *v1alpha1.ProjectionDefaultValue
		wantValue    string
		wantFound    bool
		wantError    bool
	}{
		{
			name:         "valid RFC3339 timestamp",
			resourceYaml: subaccountCR,
			path:         "status.conditions[?(@.type=='Synced')].lastTransitionTime",
			valueType:    v1alpha1.TypeTimestamp,
			wantValue:    "1757692661",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "non-string field returns error",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.intValue",
			valueType:    v1alpha1.TypeTimestamp,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "non-timestamp string returns error",
			resourceYaml: subaccountCR,
			path:         "metadata.name",
			valueType:    v1alpha1.TypeTimestamp,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
	})
}

func TestParseProjectionValue(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValue int64
		wantError bool
	}{
		{
			name:      "plain integer",
			input:     "42",
			wantValue: 42,
		},
		{
			name:      "negative integer",
			input:     "-1",
			wantValue: -1,
		},
		{
			name:      "RFC3339 timestamp",
			input:     "2025-09-12T15:57:41Z",
			wantValue: 1757692661,
		},
		{
			name:      "invalid string",
			input:     "not-a-number",
			wantError: true,
		},
		{
			name:      "empty string",
			input:     "",
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseProjectionValue(tt.input)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.wantValue, got)
			}
		})
	}
}

func TestAggregateGroupValue(t *testing.T) {
	valueByUID := map[string]int64{
		"uid-a": 10,
		"uid-b": 30,
		"uid-c": 20,
	}

	tests := []struct {
		name      string
		uids      []string
		vf        *v1alpha1.ValueFromProjection
		wantValue int64
		wantOK    bool
	}{
		{
			name:      "sum (default)",
			uids:      []string{"uid-a", "uid-b", "uid-c"},
			vf:        &v1alpha1.ValueFromProjection{Aggregation: ""},
			wantValue: 60,
			wantOK:    true,
		},
		{
			name:      "explicit sum",
			uids:      []string{"uid-a", "uid-b", "uid-c"},
			vf:        &v1alpha1.ValueFromProjection{Aggregation: v1alpha1.AggregationSum},
			wantValue: 60,
			wantOK:    true,
		},
		{
			name:      "max",
			uids:      []string{"uid-a", "uid-b", "uid-c"},
			vf:        &v1alpha1.ValueFromProjection{Aggregation: v1alpha1.AggregationMax},
			wantValue: 30,
			wantOK:    true,
		},
		{
			name:      "min",
			uids:      []string{"uid-a", "uid-b", "uid-c"},
			vf:        &v1alpha1.ValueFromProjection{Aggregation: v1alpha1.AggregationMin},
			wantValue: 10,
			wantOK:    true,
		},
		{
			name:      "mean",
			uids:      []string{"uid-a", "uid-b", "uid-c"}, // 10+30+20=60, 60/3=20
			vf:        &v1alpha1.ValueFromProjection{Aggregation: v1alpha1.AggregationMean},
			wantValue: 20,
			wantOK:    true,
		},
		{
			name:      "mean floor division",
			uids:      []string{"uid-a", "uid-b"}, // 10+30=40, 40/2=20
			vf:        &v1alpha1.ValueFromProjection{Aggregation: v1alpha1.AggregationMean},
			wantValue: 20,
			wantOK:    true,
		},
		{
			name:      "single uid",
			uids:      []string{"uid-b"},
			vf:        &v1alpha1.ValueFromProjection{Aggregation: v1alpha1.AggregationMax},
			wantValue: 30,
			wantOK:    true,
		},
		{
			name:   "no matching uids",
			uids:   []string{"uid-missing"},
			vf:     &v1alpha1.ValueFromProjection{Aggregation: v1alpha1.AggregationSum},
			wantOK: false,
		},
		{
			name:   "nil vf",
			uids:   []string{"uid-a"},
			vf:     nil,
			wantOK: false,
		},
		{
			name:   "empty uids",
			uids:   []string{},
			vf:     &v1alpha1.ValueFromProjection{Aggregation: v1alpha1.AggregationSum},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := aggregateGroupValue(tt.uids, valueByUID, tt.vf)
			require.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				require.Equal(t, tt.wantValue, got)
			}
		})
	}
}

func TestResolveValueFrom(t *testing.T) {
	objYaml := `
apiVersion: test/v1
kind: TestResource
metadata:
  name: run-123
  uid: "test-uid-abc"
  creationTimestamp: "2025-09-12T15:57:41Z"
`
	var object map[string]interface{}
	require.NoError(t, yaml.Unmarshal([]byte(objYaml), &object))
	obj := unstructured.Unstructured{Object: object}
	list := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{obj}}

	vf := &v1alpha1.ValueFromProjection{
		FieldPath: "metadata.creationTimestamp",
		Type:      v1alpha1.ValueTypeTimestamp,
	}

	result := resolveValueFrom(list, vf)
	require.Len(t, result, 1)
	require.Equal(t, int64(1757692661), result["test-uid-abc"])
}

func TestResolveValueFrom_nil(t *testing.T) {
	list := &unstructured.UnstructuredList{}
	result := resolveValueFrom(list, nil)
	require.Empty(t, result)
}

func TestResolveValueFrom_default(t *testing.T) {
	var object map[string]interface{}
	require.NoError(t, yaml.Unmarshal([]byte(`
apiVersion: test/v1
kind: TestResource
metadata:
  name: run-no-ts
  uid: "uid-no-ts"
`), &object))
	list := &unstructured.UnstructuredList{
		Items: []unstructured.Unstructured{{Object: object}},
	}

	vf := &v1alpha1.ValueFromProjection{
		FieldPath: "metadata.creationTimestamp",
		Type:      v1alpha1.ValueTypeInteger,
		Default:   v1alpha1.NewProjectionDefaultValue("99"),
	}

	result := resolveValueFrom(list, vf)
	require.Len(t, result, 1)
	require.Equal(t, int64(99), result["uid-no-ts"])
}

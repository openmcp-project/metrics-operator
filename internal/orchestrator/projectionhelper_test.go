package orchestrator

import (
	"encoding/json"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
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
	valueType    Type
	wantValue    string
	wantFound    bool
	wantError    bool
}) {
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := toUnstructured(t, tt.resourceYaml)
			value, ok, err := nestedFieldValue(obj, tt.path, tt.valueType)

			if (err != nil) != tt.wantError {
				t.Errorf("unexpected error: got %v, wantErr %v", err, tt.wantError)
			}
			if ok != tt.wantFound {
				t.Errorf("unexpected ok result: got %v, want %v", ok, tt.wantFound)
			}
			// For complex types (like maps), parse both JSON strings and compare the objects
			if tt.valueType == TypeMap && value != "" && tt.wantValue != "" {
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
		valueType    Type
		wantValue    string
		wantFound    bool
		wantError    bool
	}{
		{
			name:         "top level value retrieval",
			resourceYaml: subaccountCR,
			path:         "kind",
			valueType:    TypePrimitive,
			wantValue:    "NopResource",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with name selector",
			resourceYaml: subaccountCR,
			path:         "spec.deletionPolicy",
			valueType:    TypePrimitive,
			wantValue:    "Delete",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with escaped name selector",
			resourceYaml: subaccountCR,
			path:         "metadata.annotations.crossplane\\.io/external-name",
			valueType:    TypePrimitive,
			wantValue:    "ext-example",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with index selector",
			resourceYaml: subaccountCR,
			path:         "status.conditions[1].status",
			valueType:    TypePrimitive,
			wantValue:    "True",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with filter selector",
			resourceYaml: subaccountCR,
			path:         "status.conditions[?(@.type=='Ready')].status",
			valueType:    TypePrimitive,
			wantValue:    "True",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with array slice selector",
			resourceYaml: subaccountCR,
			path:         "status.conditions[0:1].status",
			valueType:    TypePrimitive,
			wantValue:    "True",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with wildcard selector; collection results are not supported",
			resourceYaml: subaccountCR,
			path:         "status.conditions[*].status",
			valueType:    TypePrimitive,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "non-existent value",
			resourceYaml: subaccountCR,
			path:         "metadata.labels.nonexistent",
			valueType:    TypePrimitive,
			wantValue:    "",
			wantFound:    false,
			wantError:    false,
		},
		{
			name:         "nested non-string value retrieval with default print format",
			resourceYaml: subaccountCR,
			path:         "status.conditions[0].observedGeneration",
			valueType:    TypePrimitive,
			wantValue:    "1",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "retrieval of collection types is not supported for primitive",
			resourceYaml: subaccountCR,
			path:         "status.conditions[0]",
			valueType:    TypePrimitive,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "invalid array index returns an error",
			resourceYaml: subaccountCR,
			path:         "status.conditions[abc].status",
			valueType:    TypePrimitive,
			wantValue:    "",
			wantFound:    false,
			wantError:    true,
		},
		{
			name:         "invalid path syntax returns an error",
			resourceYaml: subaccountCR,
			path:         "$.[status.conditions[0].status]",
			valueType:    TypePrimitive,
			wantValue:    "",
			wantFound:    false,
			wantError:    true,
		},
		{
			name:         "empty type defaults to primitive",
			resourceYaml: subaccountCR,
			path:         "kind",
			valueType:    "",
			wantValue:    "NopResource",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypePrimitive handles boolean value",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.boolValue",
			valueType:    TypePrimitive,
			wantValue:    "true",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypePrimitive handles integer value",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.intValue",
			valueType:    TypePrimitive,
			wantValue:    "42",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypePrimitive handles float value",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.floatValue",
			valueType:    TypePrimitive,
			wantValue:    "3.14",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypePrimitive on map field should error",
			resourceYaml: subaccountCR,
			path:         "metadata.labels",
			valueType:    TypePrimitive,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypePrimitive on array field should error",
			resourceYaml: subaccountCR,
			path:         "status.conditions",
			valueType:    TypePrimitive,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypePrimitive on null value",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.nullValue",
			valueType:    TypePrimitive,
			wantValue:    "null",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "root path with TypePrimitive should error",
			resourceYaml: subaccountCR,
			path:         ".",
			valueType:    TypePrimitive,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "filter returning no results",
			resourceYaml: subaccountCR,
			path:         "status.conditions[?(@.type=='NonExistent')]",
			valueType:    TypePrimitive,
			wantValue:    "",
			wantFound:    false,
			wantError:    false,
		},
		{
			name:         "TypePrimitive on empty string field",
			resourceYaml: `{"metadata":{"name":""}}`,
			path:         "metadata.name",
			valueType:    TypePrimitive,
			wantValue:    "",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "Deeply nested non-existent path",
			resourceYaml: subaccountCR,
			path:         "spec.missing.deeply.nested.path",
			valueType:    TypePrimitive,
			wantValue:    "",
			wantFound:    false,
			wantError:    false,
		},
	}
	runTests(t, tests)
}

// TestNestedPrimitiveValue_Map tests the TypeMap functionality
func TestNestedFieldValue_map(t *testing.T) {
	tests := []struct {
		name         string
		resourceYaml string
		path         string
		valueType    Type
		wantValue    string
		wantFound    bool
		wantError    bool
	}{
		{
			name:         "TypeMap on map field should succeed",
			resourceYaml: subaccountCR,
			path:         "metadata.labels",
			valueType:    TypeMap,
			wantValue:    `{"app":"myapp","env":"prod","team":"platform"}`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeMap on nested map should succeed",
			resourceYaml: subaccountCR,
			path:         "spec.forProvider.config",
			valueType:    TypeMap,
			wantValue:    `{"nested":"value"}`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeMap on empty map should succeed",
			resourceYaml: `{"metadata":{"labels":{}}}`,
			path:         "metadata.labels",
			valueType:    TypeMap,
			wantValue:    `{}`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeMap on entire status.atProvider",
			resourceYaml: subaccountCR,
			path:         "status.atProvider",
			valueType:    TypeMap,
			wantValue:    `{"id": "12345","nullValue": null,"boolValue": true,"intValue": 42,"floatValue": 3.14}`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "root path with TypeMap should succeed",
			resourceYaml: subaccountCR,
			path:         ".",
			valueType:    TypeMap,
			wantValue:    `{"apiVersion":"nop.crossplane.io/v1alpha1","kind":"NopResource","metadata":{"annotations":{"crossplane.io/external-name":"ext-example","crossplane.io/external-create-succeeded":"2025-11-19T09:26:05Z"},"name":"example","labels":{"app":"myapp","env":"prod","team":"platform"}},"spec":{"deletionPolicy":"Delete","forProvider":{"tags":[{"name":"tag1","value":"value1"}],"emptyList":[],"config":{"nested":"value"}}},"status":{"conditions":[{"lastTransitionTime":"2025-09-12T15:57:41Z","observedGeneration":1,"reason":"ReconcileSuccess","status":"True","type":"Synced"},{"lastTransitionTime":"2025-09-09T14:33:38Z","reason":"Available","status":"True","type":"Ready"}],"emptyConditions":[],"atProvider":{"id":"12345","nullValue":null,"boolValue":true,"intValue":42,"floatValue":3.14}}}`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeMap on primitive field should error",
			resourceYaml: subaccountCR,
			path:         "kind",
			valueType:    TypeMap,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypeMap on array field should error",
			resourceYaml: subaccountCR,
			path:         "status.conditions",
			valueType:    TypeMap,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypeMap with wildcard should error on multiple results",
			resourceYaml: subaccountCR,
			path:         "status.conditions[*]",
			valueType:    TypeMap,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypeMap with slice notation should error on multiple results",
			resourceYaml: subaccountCR,
			path:         "status.conditions[0:2]",
			valueType:    TypeMap,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypeMap on null value should error",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.nullValue",
			valueType:    TypeMap,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypeMap on non-existent field",
			resourceYaml: subaccountCR,
			path:         "metadata.nonexistent",
			valueType:    TypeMap,
			wantValue:    "",
			wantFound:    false,
			wantError:    false,
		},
		{
			name:         "TypeMap on annotations",
			resourceYaml: subaccountCR,
			path:         "metadata.annotations",
			valueType:    TypeMap,
			wantValue:    `{"crossplane.io/external-name":"ext-example","crossplane.io/external-create-succeeded":"2025-11-19T09:26:05Z"}`,
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
		valueType    Type
		wantValue    string
		wantFound    bool
		wantError    bool
	}{
		{
			name:         "TypeSlice on array field should succeed",
			resourceYaml: subaccountCR,
			path:         "status.conditions",
			valueType:    TypeSlice,
			wantValue:    `[{"lastTransitionTime":"2025-09-12T15:57:41Z","observedGeneration":1,"reason":"ReconcileSuccess","status":"True","type":"Synced"},{"lastTransitionTime":"2025-09-09T14:33:38Z","reason":"Available","status":"True","type":"Ready"}]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice on empty array should succeed",
			resourceYaml: subaccountCR,
			path:         "status.emptyConditions",
			valueType:    TypeSlice,
			wantValue:    `[]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice wraps single primitive from filter",
			resourceYaml: subaccountCR,
			path:         "status.conditions[?(@.type=='Ready')].status",
			valueType:    TypeSlice,
			wantValue:    `["True"]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice wraps single map from array index",
			resourceYaml: subaccountCR,
			path:         "status.conditions[0]",
			valueType:    TypeSlice,
			wantValue:    `[{"lastTransitionTime":"2025-09-12T15:57:41Z","observedGeneration":1,"reason":"ReconcileSuccess","status":"True","type":"Synced"}]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice on direct primitive path wraps in array",
			resourceYaml: subaccountCR,
			path:         "kind",
			valueType:    TypeSlice,
			wantValue:    `["NopResource"]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice with array slice returns multiple items",
			resourceYaml: subaccountCR,
			path:         "status.conditions[0:2].type",
			valueType:    TypeSlice,
			wantValue:    `["Synced","Ready"]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice with wildcard returns all items",
			resourceYaml: subaccountCR,
			path:         "status.conditions[*].type",
			valueType:    TypeSlice,
			wantValue:    `["Synced","Ready"]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice wraps single integer",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.intValue",
			valueType:    TypeSlice,
			wantValue:    `[42]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice wraps single boolean",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.boolValue",
			valueType:    TypeSlice,
			wantValue:    `[true]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice wraps single float",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.floatValue",
			valueType:    TypeSlice,
			wantValue:    `[3.14]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice on map field wraps map in array",
			resourceYaml: subaccountCR,
			path:         "metadata.labels",
			valueType:    TypeSlice,
			wantValue:    `[{"app":"myapp","env":"prod","team":"platform"}]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "root path with TypeSlice should error",
			resourceYaml: subaccountCR,
			path:         ".",
			valueType:    TypeSlice,
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "TypeSlice on non-existent field",
			resourceYaml: subaccountCR,
			path:         "status.nonexistent",
			valueType:    TypeSlice,
			wantValue:    "",
			wantFound:    false,
			wantError:    false,
		},
		{
			name:         "TypeSlice wraps null value",
			resourceYaml: subaccountCR,
			path:         "status.atProvider.nullValue",
			valueType:    TypeSlice,
			wantValue:    `[null]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice on filter returning multiple primitives",
			resourceYaml: subaccountCR,
			path:         "status.conditions[*].status",
			valueType:    TypeSlice,
			wantValue:    `["True","True"]`,
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "TypeSlice on a field that is a slice of primitive values",
			resourceYaml: `{"metadata":{"finalizers":["f1",12]}}`,
			path:         "metadata.finalizers",
			valueType:    TypeSlice,
			wantValue:    `["f1",12]`,
			wantFound:    true,
			wantError:    false,
		},
	}

	runTests(t, tests)
}

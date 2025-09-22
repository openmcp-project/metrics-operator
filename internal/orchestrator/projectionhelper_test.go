package orchestrator

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

const subaccountCR = `
apiVersion: account.btp.sap.crossplane.io/v1alpha1
kind: Subaccount
metadata:
  annotations:
    crossplane.io/external-name: test-subaccount
  name: test-subaccount
spec:
  deletionPolicy: Delete
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
`

func TestNestedPrimitiveValue(t *testing.T) {
	tests := []struct {
		name         string
		resourceYaml string
		path         string
		wantValue    string
		wantFound    bool
		wantError    bool
	}{
		{
			name:         "top level value retrieval",
			resourceYaml: subaccountCR,
			path:         "kind",
			wantValue:    "Subaccount",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with name selector",
			resourceYaml: subaccountCR,
			path:         "spec.deletionPolicy",
			wantValue:    "Delete",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with escaped name selector",
			resourceYaml: subaccountCR,
			path:         "metadata.annotations.crossplane\\.io/external-name",
			wantValue:    "test-subaccount",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with index selector",
			resourceYaml: subaccountCR,
			path:         "status.conditions[1].status",
			wantValue:    "True",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with filter selector",
			resourceYaml: subaccountCR,
			path:         "status.conditions[?(@.type=='Ready')].status",
			wantValue:    "True",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "nested value retrieval with array slice selector",
			resourceYaml: subaccountCR,
			path:         "status.conditions[0:1].status",
			wantValue:    "True",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "non-existent value",
			resourceYaml: subaccountCR,
			path:         "metadata.labels.app",
			wantValue:    "",
			wantFound:    false,
			wantError:    false,
		},
		{
			name:         "nested non-string value retrieval with default print format",
			resourceYaml: subaccountCR,
			path:         "status.conditions[0].observedGeneration",
			wantValue:    "1",
			wantFound:    true,
			wantError:    false,
		},
		{
			name:         "retrieval of collection types is not supported",
			resourceYaml: subaccountCR,
			path:         "status.conditions[0]",
			wantValue:    "",
			wantFound:    true,
			wantError:    true,
		},
		{
			name:         "invalid array index returns an error",
			resourceYaml: subaccountCR,
			path:         "status.conditions[abc].status",
			wantValue:    "",
			wantFound:    false,
			wantError:    true,
		},
		{
			name:         "invalid path syntax returns an error",
			resourceYaml: subaccountCR,
			path:         "$.[status.conditions[0].status]",
			wantValue:    "",
			wantFound:    false,
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := toUnstructured(t, tt.resourceYaml)
			value, ok, err := nestedPrimitiveValue(obj, tt.path)

			if (err != nil) != tt.wantError {
				t.Errorf("unexpected error: got %v, wantErr %v", err, tt.wantError)
			}
			if ok != tt.wantFound {
				t.Errorf("unexpected ok result: got %v, want %v", ok, tt.wantFound)
			}
			if value != tt.wantValue {
				t.Errorf("unexpected value: got %v, want %v", value, tt.wantValue)
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

package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/samber/lo"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/jsonpath"
)

// nestedFieldValue extracts a value from an unstructured Kubernetes object using JSONPath.
//
// Returns:
//   - string: the extracted value (JSON serialized for complex types)
//   - bool: true if value was found, false if not found
//   - error: any error encountered during extraction or type validation
//
// The valueType parameter enforces type expectations:
//   - TypePrimitive: only accepts primitive values (string, number, bool)
//   - TypeSlice: indendet for slices/arrays, wraps single values in [].
//   - TypeMap: only accepts a single map object

// For primitive types, string conversion relies on the default format when printing the value.
// For complex types (maps and slices), the value is serialized to JSON String.
//
// Path format:
//   - Use dot-notation without brackets or leading dot (e.g., "metadata.name")
//   - Use "." to export the entire object as JSON (requires TypeMap)
func nestedFieldValue(obj unstructured.Unstructured, path string, valueType v1alpha1.DimensionType, defaultValue *v1alpha1.ProjectionDefaultValue) (string, bool, error) {
	if path == "." {
		if valueType != v1alpha1.TypeMap {
			return "", true, fmt.Errorf("type %s cannot be used with root path '.', only 'map' is supported", valueType)
		}
		jsonBytes, err := json.Marshal(obj.UnstructuredContent())
		if err != nil {
			return "", true, fmt.Errorf("failed to serialize object to JSON: %v", err)
		}
		return string(jsonBytes), true, nil
	}

	// Parse and execute JSONPath
	jp := jsonpath.New("projection").AllowMissingKeys(true)
	if err := jp.Parse(fmt.Sprintf("{.%s}", path)); err != nil {
		return "", false, fmt.Errorf("failed to parse path: %v", err)
	}

	results, err := jp.FindResults(obj.UnstructuredContent())
	if err != nil {
		return "", false, fmt.Errorf("failed to find results: %v", err)
	}

	// Value not found
	if len(results) == 0 || len(results[0]) == 0 {
		if defaultValue != nil {
			defaultAsString, err := defaultValue.AsString(valueType)
			if err != nil {
				return "", false, fmt.Errorf("failed to parse default value: %v", err)
			}
			return defaultAsString, true, nil
		}
		return "", false, nil
	}

	// Validate single value for non-slice types
	if valueType != v1alpha1.TypeSlice && (len(results) > 1 || len(results[0]) > 1) {
		return "", true, fmt.Errorf("fieldPath matches more than one value, which is not supported for type %s", valueType)
	}

	// Handle each type
	switch valueType {
	case v1alpha1.TypeSlice:
		var values []interface{}
		for _, result := range results[0] {
			values = append(values, result.Interface())
		}

		// Multiple items - marshal as array
		if len(values) > 1 {
			jsonBytes, err := json.Marshal(values)
			if err != nil {
				return "", true, fmt.Errorf("failed to marshal slice to JSON: %v", err)
			}
			return string(jsonBytes), true, nil
		}

		// Single item - check if it's already a slice or needs wrapping
		if len(values) == 1 {
			switch v := values[0].(type) {
			case []interface{}:
				// Already a slice, marshal directly
				jsonBytes, err := json.Marshal(v)
				if err != nil {
					return "", true, fmt.Errorf("failed to marshal slice to JSON: %v", err)
				}
				return string(jsonBytes), true, nil
			default:
				// Wrap single non-slice item in an array
				jsonBytes, err := json.Marshal([]interface{}{v})
				if err != nil {
					return "", true, fmt.Errorf("failed to marshal slice to JSON: %v", err)
				}
				return string(jsonBytes), true, nil
			}
		}

		// Empty results
		return "[]", true, nil

	case v1alpha1.TypePrimitive:
		value := results[0][0].Interface()

		// Reject collection types
		switch value.(type) {
		case map[string]interface{}, []interface{}:
			return "", true, errors.New("fieldPath results in collection type which is not supported for type primitive")
		}

		if value == nil {
			return "null", true, nil
		}
		return fmt.Sprintf("%v", value), true, nil

	case v1alpha1.TypeMap:
		value := results[0][0].Interface()

		if _, ok := value.(map[string]interface{}); !ok {
			return "", true, errors.New("fieldPath does not result in a map type")
		}

		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return "", true, fmt.Errorf("failed to marshal map to JSON: %v", err)
		}
		return string(jsonBytes), true, nil

	default:
		return "", false, fmt.Errorf("unsupported type: %s", valueType)
	}
}

type projectionGroups map[string][][]projectedField

// extractProjectionGroupsFrom takes a list of unstructured objects and a list of projections,
// It returns a map where the key is a unique combination of projected values and the value is a list of groups of projected fields that share that combination.
func extractProjectionGroupsFrom(list *unstructured.UnstructuredList, projections []v1alpha1.Projection) projectionGroups {
	collection := make([][]projectedField, 0, len(list.Items))

	for _, obj := range list.Items {
		var fields []projectedField
		for _, projection := range projections {
			if projection.Name != "" && projection.FieldPath != "" {
				name := projection.Name
				value, found, err := nestedFieldValue(obj, projection.FieldPath, v1alpha1.DimensionType(projection.Type))
				fields = append(fields, projectedField{name: name, value: value, found: found, error: err})
			}
		}
		if fields != nil {
			collection = append(collection, fields)
		}
	}

	// Group by the combination of all projected values (cartesian product)
	groups := lo.GroupBy(collection, func(fields []projectedField) string {
		keyParts := make([]string, 0, len(fields))
		for _, f := range fields {
			keyParts = append(keyParts, f.GetID())
		}
		return strings.Join(keyParts, ",")
	})

	return groups
}

package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/jsonpath"
)

// Type represents the possible types for dimension values
type Type string

const (
	TypePrimitive Type = "primitive"
	TypeSlice     Type = "slice"
	TypeMap       Type = "map"
)

// nestedFieldValue returns a string value based on the result of the client-go JSONPath parser.
// Returns false if the value is not found.
// Returns an error if the value type doesn't match the expected resultType.
// Returns an error if the given path can't be parsed.
//
// The resultType parameter enforces type expectations:
// - ResultTypePrimitive: only accepts primitive values (string, number, bool) - errors on maps/slices
// - ResultTypeSlice: only accepts slices/arrays - wraps single results in [] if the path can return multiple items
// - ResultTypeMap: only accepts a single map object - errors on slices or multiple results
//
// For primitive types, string conversion relies on the default format when printing the value.
// For complex types (maps and slices), the value is serialized to JSON String.
// The input path is expected to be passed in dot-notation without brackets or a leading dot.
// Use "." to export the entire object as a JSON string.
func nestedFieldValue(obj unstructured.Unstructured, path string, vType ...Type) (string, bool, error) {
	// TODO: make valueType not optional in funciton calls
	valueType := TypePrimitive
	if len(vType) > 0 {
		valueType = vType[0]
	}
	if valueType == "" {
		valueType = TypePrimitive
	}

	if path == "." {
		if valueType != TypeMap {
			return "", true, fmt.Errorf("type %s cannot be used with root path '.'", valueType)
		}
		jsonBytes, err := json.Marshal(obj.UnstructuredContent())
		if err != nil {
			return "", true, fmt.Errorf("failed to serialize object to JSON: %v", err)
		}
		return string(jsonBytes), true, nil

	}

	jp := jsonpath.New("projection").AllowMissingKeys(true)
	if err := jp.Parse(fmt.Sprintf("{.%s}", path)); err != nil {
		return "", false, fmt.Errorf("failed to parse path: %v", err)
	}
	results, err := jp.FindResults(obj.UnstructuredContent())
	if err != nil {
		return "", false, fmt.Errorf("failed to find results: %v", err)
	}
	if len(results) == 0 || len(results[0]) == 0 {
		return "", false, nil
	}
	if valueType != TypeSlice && (len(results) > 1 || len(results[0]) > 1) {
		return "", true, fmt.Errorf("fieldPath matches more than one value which is not supported for type %s", valueType)
	}

	// Handle slice type
	if valueType == TypeSlice {
		var values []interface{}
		for _, result := range results[0] {
			values = append(values, result.Interface())
		}

		// If we have multiple items, marshal as array
		if len(values) > 1 {
			jsonBytes, err := json.Marshal(values)
			if err != nil {
				return "", true, fmt.Errorf("failed to marshal slice to JSON: %v", err)
			}
			return string(jsonBytes), true, nil
		}

		// For single item, check if it's already a slice/array
		if len(values) == 1 {
			firstItem := values[0]
			switch v := firstItem.(type) {
			case []interface{}:
				// Already a slice, marshal directly
				jsonBytes, err := json.Marshal(v)
				if err != nil {
					return "", true, fmt.Errorf("failed to marshal nested slice to JSON: %v", err)
				}
				return string(jsonBytes), true, nil
			default:
				// Wrap single non-slice item in an array
				wrapped := []interface{}{v}
				jsonBytes, err := json.Marshal(wrapped)
				if err != nil {
					return "", true, fmt.Errorf("failed to marshal wrapped value to JSON: %v", err)
				}
				return string(jsonBytes), true, nil
			}
		}
	}

	// for other types, use the first result
	value := results[0][0]

	// Handle primitive type
	if valueType == TypePrimitive {
		switch value.Interface().(type) {
		case map[string]interface{}, []interface{}:
			return "", true, errors.New("fieldPath results in collection type which is not supported for type primitive")
		}
		if value.Interface() == nil {
			return "null", true, nil
		}
		return fmt.Sprintf("%v", value.Interface()), true, nil
	}

	// Handle map type
	if valueType == TypeMap {
		if _, ok := value.Interface().(map[string]interface{}); !ok {
			return "", true, errors.New("fieldPath does not result in a map type")
		}
		jsonBytes, err := json.Marshal(value.Interface())
		if err != nil {
			return "", true, fmt.Errorf("failed to marshal map to JSON: %v", err)
		}
		return string(jsonBytes), true, nil
	}

	return "", false, fmt.Errorf("unsupported type: %s", valueType)
}

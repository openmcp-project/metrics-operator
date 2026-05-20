package orchestrator

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

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
//   - TypeTimestamp: parses an RFC3339 string and returns Unix seconds as a string

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

	s, err := extractTypedValue(results, valueType)
	return s, true, err
}

// extractTypedValue converts JSONPath results to a string according to valueType.
func extractTypedValue(results [][]reflect.Value, valueType v1alpha1.DimensionType) (string, error) {
	switch valueType {
	case v1alpha1.TypeSlice:
		values := make([]interface{}, 0, len(results[0]))
		for _, result := range results[0] {
			values = append(values, result.Interface())
		}

		// Multiple items - marshal as array
		if len(values) > 1 {
			jsonBytes, err := json.Marshal(values)
			if err != nil {
				return "", fmt.Errorf("failed to marshal slice to JSON: %v", err)
			}
			return string(jsonBytes), nil
		}

		// Single item - check if it's already a slice or needs wrapping
		if len(values) == 1 {
			switch v := values[0].(type) {
			case []interface{}:
				jsonBytes, err := json.Marshal(v)
				if err != nil {
					return "", fmt.Errorf("failed to marshal slice to JSON: %v", err)
				}
				return string(jsonBytes), nil
			default:
				jsonBytes, err := json.Marshal([]interface{}{v})
				if err != nil {
					return "", fmt.Errorf("failed to marshal slice to JSON: %v", err)
				}
				return string(jsonBytes), nil
			}
		}

		return "[]", nil

	case v1alpha1.TypePrimitive:
		value := results[0][0].Interface()

		switch value.(type) {
		case map[string]interface{}, []interface{}:
			return "", errors.New("fieldPath results in collection type which is not supported for type primitive")
		}

		if value == nil {
			return "null", nil
		}
		return fmt.Sprintf("%v", value), nil

	case v1alpha1.TypeTimestamp:
		value := results[0][0].Interface()

		str, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("fieldPath does not result in a string for type timestamp, got %T", value)
		}
		t, err := time.Parse(time.RFC3339, str)
		if err != nil {
			return "", fmt.Errorf("failed to parse timestamp %q: %v", str, err)
		}
		return strconv.FormatInt(t.Unix(), 10), nil

	case v1alpha1.TypeInteger:
		value := results[0][0].Interface()

		switch v := value.(type) {
		case int64:
			return strconv.FormatInt(v, 10), nil
		case int32:
			return strconv.FormatInt(int64(v), 10), nil
		case float64:
			if v != float64(int64(v)) {
				return "", fmt.Errorf("fieldPath results in float value %v which cannot be represented as an integer", v)
			}
			return strconv.FormatInt(int64(v), 10), nil
		case string:
			if _, err := strconv.ParseInt(v, 10, 64); err != nil {
				return "", fmt.Errorf("fieldPath results in string %q which cannot be parsed as an integer", v)
			}
			return v, nil
		default:
			return "", fmt.Errorf("fieldPath does not result in an integer for type integer, got %T", value)
		}

	case v1alpha1.TypeMap:
		value := results[0][0].Interface()

		if _, ok := value.(map[string]interface{}); !ok {
			return "", errors.New("fieldPath does not result in a map type")
		}

		jsonBytes, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("failed to marshal map to JSON: %v", err)
		}
		return string(jsonBytes), nil

	default:
		return "", fmt.Errorf("unsupported type: %s", valueType)
	}
}

type projectionGroups map[string][][]projectedField

// parseProjectionValue converts a projected value string to int64.
// It tries integer parsing first, then RFC3339 timestamp parsing.
func parseProjectionValue(s string) (int64, error) {
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Unix(), nil
	}
	return 0, fmt.Errorf("cannot parse %q as integer or RFC3339 timestamp", s)
}

// resolveValueFrom resolves the valueFrom projection for each object in the list,
// returning a map from object UID to the resolved int64 gauge value.
// Objects where valueFrom cannot be resolved are omitted from the map.
func resolveValueFrom(list *unstructured.UnstructuredList, vf *v1alpha1.ValueFromProjection) map[string]int64 {
	result := make(map[string]int64)
	if vf == nil || vf.FieldPath == "" {
		return result
	}
	valueType := vf.Type
	if valueType == "" {
		valueType = v1alpha1.ValueTypeInteger
	}
	// ValueType maps to DimensionType for nestedFieldValue; both integer and timestamp
	// store their default as a JSON-encoded primitive string.
	dimType := v1alpha1.DimensionType(valueType)
	for _, obj := range list.Items {
		raw, found, err := nestedFieldValue(obj, vf.FieldPath, dimType, vf.Default)
		if err != nil || !found || raw == "" {
			continue
		}
		v, err := parseProjectionValue(raw)
		if err != nil {
			continue
		}
		result[string(obj.GetUID())] = v
	}
	return result
}

// aggregateGroupValue combines per-object values for a group of UIDs using the
// aggregation function specified on the ValueFromProjection.
// Returns (value, ok) — ok is false if no UIDs had a resolved value.
func aggregateGroupValue(uids []string, valueByUID map[string]int64, vf *v1alpha1.ValueFromProjection) (int64, bool) {
	if vf == nil {
		return 0, false
	}
	agg := vf.Aggregation
	if agg == "" {
		agg = v1alpha1.AggregationSum
	}

	var sum int64
	var count int64
	var result int64
	found := false
	for _, uid := range uids {
		v, ok := valueByUID[uid]
		if !ok {
			continue
		}
		sum += v
		count++
		if !found {
			result = v
			found = true
			continue
		}
		switch agg {
		case v1alpha1.AggregationMax:
			if v > result {
				result = v
			}
		case v1alpha1.AggregationMin:
			if v < result {
				result = v
			}
		}
	}
	if !found {
		return 0, false
	}
	switch agg {
	case v1alpha1.AggregationMean:
		return sum / count, true
	case v1alpha1.AggregationMax, v1alpha1.AggregationMin:
		return result, true
	default: // sum
		return sum, true
	}
}

// It returns a map where the key is a unique combination of projected values and the value is a list of groups of projected fields that share that combination.
func extractProjectionGroupsFrom(list *unstructured.UnstructuredList, projections []v1alpha1.Projection) projectionGroups {
	collection := make([][]projectedField, 0, len(list.Items))

	for _, obj := range list.Items {
		uid := string(obj.GetUID())
		var fields []projectedField
		for _, projection := range projections {
			if projection.Name != "" && projection.FieldPath != "" {
				name := projection.Name
				value, found, err := nestedFieldValue(obj, projection.FieldPath, projection.Type, projection.Default)
				fields = append(fields, projectedField{uid: uid, name: name, value: value, found: found, error: err})
			}
		}
		if fields != nil {
			collection = append(collection, fields)
		}
	}

	// Group by the combination of all projected dimension values
	groups := lo.GroupBy(collection, func(fields []projectedField) string {
		keyParts := make([]string, 0, len(fields))
		for _, f := range fields {
			keyParts = append(keyParts, f.GetID())
		}
		return strings.Join(keyParts, ",")
	})

	return groups
}

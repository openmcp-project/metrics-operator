package orchestrator

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/jsonpath"
)

// nestedPrimitiveValue returns a string value based on the result of the client-go JSONPath parser.
// Returns false if the value is not found.
// Returns an error if the value is ambiguous or a collection type.
// Returns an error if the given path can't be parsed.
//
// String conversion of non-string primitives relies on the default format when printing the value.
// The input path is expected to be passed in dot-notation without brackets or a leading dot.
// The implementation is based on similar internal client-go jsonpath usages, like kubectl
func nestedPrimitiveValue(obj unstructured.Unstructured, path string) (string, bool, error) {
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
	if len(results) > 1 || len(results[0]) > 1 {
		return "", true, errors.New("fieldPath matches more than one value which is not supported")
	}
	value := results[0][0]
	switch value.Interface().(type) {
	case map[string]interface{}, []interface{}:
		return "", true, errors.New("fieldPath results in collection type which is not supported")
	}
	return fmt.Sprintf("%v", value.Interface()), true, nil
}

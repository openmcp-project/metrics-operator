package v1alpha1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// StatusTrue indicates the metric resource is considered ready/active.
	StatusTrue = "True"
	// StatusFalse indicates the metric resource is not ready/active.
	StatusFalse = "False"
)

// GroupVersionKind defines the group, version and kind of the object that should be instrumented
type GroupVersionKind struct {
	// Define the kind of the object that should be instrumented
	Kind string `json:"kind,omitempty"`
	// Define the group of your object that should be instrumented
	Group string `json:"group,omitempty"`
	// Define version of the object you want to be instrumented
	Version string `json:"version,omitempty"`
}

// GVK returns the schema.GroupVersionKind object of v1alpha1 GVK
func (gvk *GroupVersionKind) GVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   gvk.Group,
		Kind:    gvk.Kind,
		Version: gvk.Version,
	}
}

// Projection defines the projection of the metric
type Projection struct {
	// Define the name of the field that should be extracted
	Name string `json:"name,omitempty"`

	// Define the path to the field that should be extracted
	FieldPath string `json:"fieldPath,omitempty"`

	// Type specifies the type of the projections's value.
	// It can be "primitive", "slice", "map", or "timestamp".
	// Use "timestamp" for RFC3339 time fields — the value is converted to Unix seconds.
	// If not specified, it will default to "primitive".
	// +optional
	// +default="primitive"
	// +kubebuilder:validation:Enum=primitive;slice;map;timestamp
	Type DimensionType `json:"type,omitempty"`

	// Default specifies a default value for the projection.
	// The default value is used when the specified field is not found or is null in the observed object.
	// The type is determined by the Type field.
	// If Type is "primitive", Default should be a JSON-encoded string.
	// If Type is "slice", Default should be a JSON-encoded array.
	// If Type is "map", Default should be a JSON-encoded object.
	// +optional
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Default *ProjectionDefaultValue `json:"default,omitempty"`
}

// ValueType represents the type of a gauge metric value extracted from a resource field.
type ValueType string

const (
	// ValueTypeInteger interprets the field as an integer.
	ValueTypeInteger ValueType = "integer"
	// ValueTypeTimestamp interprets the field as an RFC3339 timestamp, converting it to Unix seconds.
	ValueTypeTimestamp ValueType = "timestamp"
)

// AggregationType represents the aggregation function applied to valueFrom when multiple
// objects share the same label dimensions.
type AggregationType string

const (
	// AggregationSum sums all values in the group. This is the default.
	AggregationSum AggregationType = "sum"
	// AggregationMax takes the maximum value in the group.
	AggregationMax AggregationType = "max"
	// AggregationMin takes the minimum value in the group.
	AggregationMin AggregationType = "min"
	// AggregationMean takes the arithmetic mean (floor division) of all values in the group.
	AggregationMean AggregationType = "mean"
)

// ValueFromProjection defines a field whose value is used as the gauge metric value.
type ValueFromProjection struct {
	// Define the path to the field that should be extracted
	FieldPath string `json:"fieldPath,omitempty"`

	// Type specifies the type of the field's value.
	// Use "integer" for numeric fields — the value is used directly as the gauge value.
	// Use "timestamp" for RFC3339 time fields — the value is converted to Unix seconds.
	// If not specified, it will default to "integer".
	// +optional
	// +default="integer"
	// +kubebuilder:validation:Enum=integer;timestamp
	Type ValueType `json:"type,omitempty"`

	// Aggregation specifies how values are combined when multiple objects share the same
	// label dimensions. It can be "sum", "max", "min", or "mean". Defaults to "sum".
	// +optional
	// +default="sum"
	// +kubebuilder:validation:Enum=sum;max;min;mean
	Aggregation AggregationType `json:"aggregation,omitempty"`

	// Default specifies a fallback value used when the field specified by fieldPath is
	// not found or null on a resource. Must be parseable according to Type:
	// an integer string for "integer", or an RFC3339 timestamp for "timestamp".
	// +optional
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Default *ProjectionDefaultValue `json:"default,omitempty"`
}

// ProjectionDefaultValue is a wrapper around json.RawMessage to allow flexible default values for projections.
type ProjectionDefaultValue struct {
	json.RawMessage
}

func NewProjectionDefaultValue(value interface{}) *ProjectionDefaultValue {
	pdv := &ProjectionDefaultValue{}
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	pdv.RawMessage = jsonBytes
	return pdv
}

func (pdv *ProjectionDefaultValue) AsString(valueType DimensionType) (string, error) {
	switch valueType {
	case TypePrimitive, TypeTimestamp, TypeInteger:
		var strValue string
		if err := json.Unmarshal(pdv.RawMessage, &strValue); err != nil {
			return "", err
		}
		return strValue, nil
	case TypeSlice, TypeMap:
		return string(pdv.RawMessage), nil
	default:
		return "", fmt.Errorf("unsupported dimension type: %s", valueType)
	}
}

// Dimension defines the dimension of the metric
type Dimension struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// Type represents the possible types for dimension values
type DimensionType string

const (
	TypePrimitive DimensionType = "primitive"
	TypeSlice     DimensionType = "slice"
	TypeMap       DimensionType = "map"
	TypeTimestamp DimensionType = "timestamp"
	TypeInteger   DimensionType = "integer"
)

// MetricObservation represents the latest available observation of an object's state
type MetricObservation struct {
	// The timestamp of the observation
	Timestamp metav1.Time `json:"timestamp,omitempty"`

	// The latest value of the metric
	LatestValue string `json:"latestValue,omitempty"`

	Dimensions []Dimension `json:"dimensions,omitempty"`
}

// GetTimestamp returns the timestamp of the observation
func (mo *MetricObservation) GetTimestamp() metav1.Time {
	return mo.Timestamp
}

// GetValue returns the latest value of the metric
func (mo *MetricObservation) GetValue() string {
	return mo.LatestValue
}

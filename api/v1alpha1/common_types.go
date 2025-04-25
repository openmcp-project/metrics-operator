package v1alpha1

import (
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
}

// Dimension defines the dimension of the metric
type Dimension struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

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

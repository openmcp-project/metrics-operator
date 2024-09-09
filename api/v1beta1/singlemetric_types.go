/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum=Ready;Failed;Pending
type PhaseType string

const (
	PhaseActive  PhaseType = "Ready"
	PhaseFailed  PhaseType = "Failed"
	PhasePending PhaseType = "Pending"
)

// SingleMetricSpec defines the desired state of SingleMetric
type SingleMetricSpec struct {
	// Sets the name that will be used to identify the metric in Dynatrace(or other sinks)
	Name string `json:"name,omitempty"`
	// Sets the description that will be used to identify the metric in Dynatrace(or other sinks)
	// +optional
	Description string `json:"description,omitempty"`
	// Decide which kind the metric should keep track of (needs to be plural version)

	// +kubebuilder:validation:Required
	Target GroupVersionKind `json:"target,omitempty"`

	// +optional
	Selectors Selectors `json:"selectors,inline"`

	// Define in what interval the query should be recorded (in minutes) # min: 1
	// +optional
	// +kubebuilder:default:=720
	// +kubebuilder:validation:Minimum=1
	Frequency int `json:"frequency,omitempty"`

	ClusterAccessFacade `json:",inline"`
}

type Selectors struct {
	// Define labels of your object to adapt filters of the query
	// +optional
	LabelSelector string `json:"labelSelector,omitempty"`
	// Define fields of your object to adapt filters of the query
	// +optional
	FieldSelector string `json:"fieldSelector,omitempty"`
}

type GroupVersionKind struct {
	// Define the kind of the object that should be instrumented
	Kind string `json:"kind,omitempty"`
	// Define the group of your object that should be instrumented
	Group string `json:"group,omitempty"`
	// Define version of the object you want to be instrumented
	Version string `json:"version,omitempty"`
}

type MetricObservation struct {
	// The timestamp of the observation
	Timestamp metav1.Time `json:"timestamp,omitempty"`

	// The latest value of the metric
	LatestValue string `json:"latestValue,omitempty"`

	Dimensions []Dimension `json:"dimensions,omitempty"`
}

func (mo *MetricObservation) GetTimestamp() metav1.Time {
	return mo.Timestamp
}

func (mo *MetricObservation) GetValue() string {
	return mo.LatestValue
}

// SingleMetricStatus defines the observed state of SingleMetric
type SingleMetricStatus struct {

	// Observation represent the latest available observation of an object's state
	Observation MetricObservation `json:"observation,omitempty"`

	// Ready is like a snapshot of the current state of the metric's lifecycle
	Ready string `json:"ready,omitempty"`

	// Conditions represent the latest available observations of an object's state
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
	LastReconcileTime *metav1.Time       `json:"lastReconcileTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="VALUE",type="string",JSONPath=".status.observation.latestValue"
// +kubebuilder:printcolumn:name="OBSERVED",type="date",JSONPath=".status.observation.timestamp"

// SingleMetric is the Schema for the singlemetrics API
type SingleMetric struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SingleMetricSpec   `json:"spec,omitempty"`
	Status SingleMetricStatus `json:"status,omitempty"`
}

func (r *SingleMetric) SetConditions(conditions ...metav1.Condition) {
	for _, c := range conditions {
		meta.SetStatusCondition(&r.Status.Conditions, c)
	}
}

func (r *SingleMetric) GvkToString() string {
	if r.Spec.Target.Group == "" {
		return fmt.Sprintf("/%s, Kind=%s", r.Spec.Target.Version, r.Spec.Target.Kind)
	}
	return fmt.Sprintf("%s/%s, Kind=%s", r.Spec.Target.Group, r.Spec.Target.Version, r.Spec.Target.Kind)
}

// +kubebuilder:object:root=true

// SingleMetricList contains a list of SingleMetric
type SingleMetricList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SingleMetric `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SingleMetric{}, &SingleMetricList{})
}

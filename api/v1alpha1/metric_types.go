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

package v1alpha1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PhaseType defines the phase of the metric
// +kubebuilder:validation:Enum=Ready;Failed;Pending
type PhaseType string

const (
	// PhaseActive represents the metric is ready
	PhaseActive PhaseType = "Ready"
	// PhaseFailed represents the metric is failed
	PhaseFailed PhaseType = "Failed"
	// PhasePending represents the metric is pending
	PhasePending PhaseType = "Pending"
)

// DataSinkReference holds a reference to a DataSink resource.
type DataSinkReference struct {
	// Name is the name of the DataSink resource.
	// +optional
	// +kubebuilder:default:="default"
	Name string `json:"name,omitempty"`
}

// MetricSpec defines the desired state of Metric
type MetricSpec struct {
	// Sets the name that will be used to identify the metric in Dynatrace(or other providers)
	Name string `json:"name,omitempty"`
	// Sets the description that will be used to identify the metric in Dynatrace(or other providers)
	// +optional
	Description string `json:"description,omitempty"`
	// +kubebuilder:validation:Required
	Target GroupVersionKind `json:"target,omitempty"`
	// Define labels of your object to adapt filters of the query
	// +optional
	LabelSelector string `json:"labelSelector,omitempty"`
	// Define fields of your object to adapt filters of the query
	// +optional
	FieldSelector string `json:"fieldSelector,omitempty"`
	// Define in what interval the query should be recorded
	// +kubebuilder:default:="10m"
	Interval metav1.Duration `json:"interval,omitempty"`

	// DataSinkRef specifies the DataSink to be used for this metric.
	// If not specified, the DataSink named "default" in the operator's
	// namespace will be used.
	// +optional
	// +kubebuilder:default:={}
	DataSinkRef *DataSinkReference `json:"dataSinkRef,omitempty"`

	// +optional
	RemoteClusterAccessRef *RemoteClusterAccessRef `json:"remoteClusterAccessRef,omitempty"`

	Projections []Projection `json:"projections,omitempty"`
}

// MetricStatus defines the observed state of ManagedMetric
type MetricStatus struct {

	// Observation represent the latest available observation of an object's state
	Observation MetricObservation `json:"observation,omitempty"`

	// Ready is like a snapshot of the current state of the metric's lifecycle
	Ready string `json:"ready,omitempty"`

	// Conditions represent the latest available observations of an object's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// Metric is the Schema for the metrics API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="VALUE",type="string",JSONPath=".status.observation.latestValue"
// +kubebuilder:printcolumn:name="OBSERVED",type="date",JSONPath=".status.observation.timestamp"
type Metric struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MetricSpec   `json:"spec,omitempty"`
	Status MetricStatus `json:"status,omitempty"`
}

// SetConditions sets the conditions of the metric
func (r *Metric) SetConditions(conditions ...metav1.Condition) {
	for _, c := range conditions {
		meta.SetStatusCondition(&r.Status.Conditions, c)
	}
}

// GvkToString returns the string representation of the metric targe GVK
func (r *Metric) GvkToString() string {
	if r.Spec.Target.Group == "" {
		return fmt.Sprintf("/%s, Kind=%s", r.Spec.Target.Version, r.Spec.Target.Kind)
	}
	return fmt.Sprintf("%s/%s, Kind=%s", r.Spec.Target.Group, r.Spec.Target.Version, r.Spec.Target.Kind)
}

// +kubebuilder:object:root=true

// MetricList contains a list of Metric
type MetricList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Metric `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Metric{}, &MetricList{})
}

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

// +kubebuilder:validation:Enum=Active;Failed;Pending
type PhaseType string

const (
	PhaseActive  PhaseType = "Active"
	PhaseFailed  PhaseType = "Failed"
	PhasePending PhaseType = "Pending"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MetricSpec defines the desired state of Metric
type MetricSpec struct {
	// Sets the name that will be used to identify the metric in Dynatrace(or other providers)
	Name string `json:"name,omitempty"`
	// Sets the description that will be used to identify the metric in Dynatrace(or other providers)
	// +optional
	Description string `json:"description,omitempty"`
	// Decide which kind the metric should keep track of (needs to be plural version)
	Kind string `json:"kind,omitempty"`
	// Define the group of your object that should be instrumented (without version at the end)
	Group string `json:"group,omitempty"`
	// Define version of the object you want to intrsument
	Version string `json:"version,omitempty"`
	// Define labels of your object to adapt filters of the query
	// +optional
	LabelSelector string `json:"labelSelector,omitempty"`
	// Define fields of your object to adapt filters of the query
	// +optional
	FieldSelector string `json:"fieldSelector,omitempty"`
	// Define in what interval the query should be recorded (in minutes) # min: 1
	// +optional
	// +kubebuilder:default:=720
	// +kubebuilder:validation:Minimum=1
	Frequency int `json:"frequency,omitempty"`

	RemoteClusterAccessFacade `json:",inline"`
}

// MetricStatus defines the observed state of ManagedMetric
type MetricStatus struct {
	// Phase is like a snapshot of the current state of the metric's lifecycle

	// +kubebuilder:default:=Pending
	Phase PhaseType `json:"phase,omitempty"`

	// Conditions represent the latest available observations of an object's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Current phase of the Metric"
// Metric is the Schema for the metrics API
type Metric struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MetricSpec   `json:"spec,omitempty"`
	Status MetricStatus `json:"status,omitempty"`
}

func (r *Metric) SetConditions(conditions ...metav1.Condition) {
	for _, c := range conditions {
		meta.SetStatusCondition(&r.Status.Conditions, c)
	}
}

func (r *Metric) GvkToString() string {
	if r.Spec.Group == "" {
		return fmt.Sprintf("/%s, Kind=%s", r.Spec.Version, r.Spec.Kind)
	}
	return fmt.Sprintf("%s/%s, Kind=%s", r.Spec.Group, r.Spec.Version, r.Spec.Kind)
}

//+kubebuilder:object:root=true

// MetricList contains a list of Metric
type MetricList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Metric `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Metric{}, &MetricList{})
}

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

// ManagedMetricSpec defines the desired state of ManagedMetric
type ManagedMetricSpec struct {
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

// ManagedMetricStatus defines the observed state of ManagedMetric
type ManagedMetricStatus struct {
	// Is set when Metric is Successfully executed and keeps track of the current cycle.
	// The cycle starts anew and the status will be set to active if execution was successfull
	// +kubebuilder:default:=Pending
	Phase PhaseType `json:"phase,omitempty"`

	// Conditions represent the latest available observations of an object's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func (r *ManagedMetric) GvkToString() string {
	if r.Spec.Group == "" {
		return fmt.Sprintf("/%s, Kind=%s", r.Spec.Version, r.Spec.Kind)
	}
	return fmt.Sprintf("%s/%s, Kind=%s", r.Spec.Group, r.Spec.Version, r.Spec.Kind)
}

func (r *ManagedMetric) SetConditions(conditions ...metav1.Condition) {
	for _, c := range conditions {
		meta.SetStatusCondition(&r.Status.Conditions, c)
	}
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Current phase of the Metric"

// ManagedMetric is the Schema for the managedmetrics API
type ManagedMetric struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ManagedMetricSpec   `json:"spec,omitempty"`
	Status ManagedMetricStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ManagedMetricList contains a list of ManagedMetric
type ManagedMetricList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManagedMetric `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManagedMetric{}, &ManagedMetricList{})
}

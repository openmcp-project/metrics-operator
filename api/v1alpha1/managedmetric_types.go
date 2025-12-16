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
	// Defines which managed resources to observe
	// +optional
	Target *GroupVersionKind `json:"target,omitempty"`
	// Defines dimensions of the metric. All specified fields must be nested strings. Nested slices are not supported.
	// If not specified, only status.conditions of the CR will be used as dimension.
	// +optional
	Dimensions map[string]string `json:"dimensions,omitempty"`
	// Define labels of your object to adapt filters of the query
	// +optional
	LabelSelector string `json:"labelSelector,omitempty"`
	// Define fields of your object to adapt filters of the query
	// +optional
	FieldSelector string `json:"fieldSelector,omitempty"`
	// Define in what interval the query should be recorded
	// +kubebuilder:default:="10m"
	Interval metav1.Duration `json:"interval,omitempty"`

	// DataSinkRef specifies the DataSink to be used for this managed metric.
	// If not specified, the DataSink named "default" in the operator's
	// namespace will be used.
	// +optional
	// +kubebuilder:default:={}
	DataSinkRef *DataSinkReference `json:"dataSinkRef,omitempty"`

	// +optional
	RemoteClusterAccessRef *RemoteClusterAccessRef `json:"remoteClusterAccessRef,omitempty"`
}

// ManagedObservation represents the latest available observation of an object's state
type ManagedObservation struct {
	// The timestamp of the observation
	Timestamp metav1.Time `json:"timestamp,omitempty"`

	// Number of resources of the managed metric (i.e. how many managed resource are there that match the query)
	Resources string `json:"resources,omitempty"`
}

// GetTimestamp returns the timestamp of the observation
func (mo *ManagedObservation) GetTimestamp() metav1.Time {
	return mo.Timestamp
}

// GetValue returns the value of the observation
func (mo *ManagedObservation) GetValue() string {
	return mo.Resources
}

// ManagedMetricStatus defines the observed state of ManagedMetric
type ManagedMetricStatus struct {

	// Observation represent the latest available observation of an object's state
	Observation ManagedObservation `json:"observation,omitempty"`

	// Is set when Metric is Successfully executed and keeps track of the current cycle.
	// The cycle starts anew and the status will be set to active if execution was successful
	Ready string `json:"ready,omitempty"`

	// Conditions represent the latest available observations of an object's state
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// GvkToString returns group, version and kind as a string
func (r *ManagedMetric) GvkToString() string {
	if r.Spec.Target != nil {
		return r.Spec.Target.GVK().String()
	}
	return ""
}

// SetConditions sets the conditions for the ManagedMetric
func (r *ManagedMetric) SetConditions(conditions ...metav1.Condition) {
	for _, c := range conditions {
		meta.SetStatusCondition(&r.Status.Conditions, c)
	}
}

// ManagedMetric is the Schema for the managedmetrics API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="VALUE",type="string",JSONPath=".status.observation.resources"
// +kubebuilder:printcolumn:name="OBSERVED",type="date",JSONPath=".status.observation.timestamp"
type ManagedMetric struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ManagedMetricSpec   `json:"spec,omitempty"`
	Status ManagedMetricStatus `json:"status,omitempty"`
}

// ManagedMetricList contains a list of ManagedMetric
// +kubebuilder:object:root=true
type ManagedMetricList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManagedMetric `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManagedMetric{}, &ManagedMetricList{})
}

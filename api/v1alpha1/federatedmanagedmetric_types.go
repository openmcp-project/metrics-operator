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

// FederatedManagedMetricSpec defines the desired state of FederatedManagedMetric
type FederatedManagedMetricSpec struct {
	Name string `json:"name,omitempty"`

	// +optional
	Description string `json:"description,omitempty"`

	// Define labels of your object to adapt filters of the query
	// +optional
	LabelSelector string `json:"labelSelector,omitempty"`
	// Define fields of your object to adapt filters of the query
	// +optional
	FieldSelector string `json:"fieldSelector,omitempty"`

	// Projections []Projection `json:"projections,omitempty"`

	// Define in what interval the query should be recorded
	// +kubebuilder:default:="10m"
	Interval metav1.Duration `json:"interval,omitempty"`

	// DataSinkRef specifies the DataSink to be used for this federated managed metric.
	// If not specified, the DataSink named "default" in the operator's
	// namespace will be used.
	// +optional
	DataSinkRef *DataSinkReference `json:"dataSinkRef,omitempty"`

	FederatedClusterAccessRef FederateClusterAccessRef `json:"federateClusterAccessRef,omitempty"`
}

// FederatedManagedMetricStatus defines the observed state of FederatedManagedMetric
type FederatedManagedMetricStatus struct {
	Observation FederatedObservation `json:"observation,omitempty"`

	// Ready is like a snapshot of the current state of the metric's lifecycle
	Ready string `json:"ready,omitempty"`

	// Conditions represent the latest available observations of an object's state
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
	LastReconcileTime *metav1.Time       `json:"lastReconcileTime,omitempty"`
}

// SetConditions sets the conditions of the FederatedManagedMetric
func (r *FederatedManagedMetric) SetConditions(conditions ...metav1.Condition) {
	for _, c := range conditions {
		meta.SetStatusCondition(&r.Status.Conditions, c)
	}
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// FederatedManagedMetric is the Schema for the federatedmanagedmetrics API
type FederatedManagedMetric struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FederatedManagedMetricSpec   `json:"spec,omitempty"`
	Status FederatedManagedMetricStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FederatedManagedMetricList contains a list of FederatedManagedMetric
type FederatedManagedMetricList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FederatedManagedMetric `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FederatedManagedMetric{}, &FederatedManagedMetricList{})
}

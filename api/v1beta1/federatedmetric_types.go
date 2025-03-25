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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FederatedMetricSpec defines the desired state of FederatedMetric
type FederatedMetricSpec struct {
	Name string `json:"name,omitempty"`

	// +optional
	Description string `json:"description,omitempty"`

	// +kubebuilder:validation:Required
	Target GroupVersionResource `json:"target,omitempty"`

	// Define labels of your object to adapt filters of the query
	// +optional
	LabelSelector string `json:"labelSelector,omitempty"`
	// Define fields of your object to adapt filters of the query
	// +optional
	FieldSelector string `json:"fieldSelector,omitempty"`

	Projections []Projection `json:"projections,omitempty"`

	// Define in what interval the query should be recorded
	// +kubebuilder:default:="12h"
	CheckInterval metav1.Duration `json:"checkInterval,omitempty"`

	FederateCAFacade `json:",inline"`
}

// FederatedObservation represents the latest available observation of an object's state
type FederatedObservation struct {
	ActiveCount  int `json:"activeCount,omitempty"`
	FailedCount  int `json:"failedCount,omitempty"`
	PendingCount int `json:"pendingCount,omitempty"`
}

// FederatedMetricStatus defines the observed state of FederatedMetric
type FederatedMetricStatus struct {
	Observation FederatedObservation `json:"observation,omitempty"`

	// Ready is like a snapshot of the current state of the metric's lifecycle
	Ready string `json:"ready,omitempty"`

	// Conditions represent the latest available observations of an object's state
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
	LastReconcileTime *metav1.Time       `json:"lastReconcileTime,omitempty"`
}

// SetConditions sets the conditions of the FederatedMetric
func (r *FederatedMetric) SetConditions(conditions ...metav1.Condition) {
	for _, c := range conditions {
		meta.SetStatusCondition(&r.Status.Conditions, c)
	}
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// FederatedMetric is the Schema for the federatedmetrics API
type FederatedMetric struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FederatedMetricSpec   `json:"spec,omitempty"`
	Status FederatedMetricStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FederatedMetricList contains a list of FederatedMetric
type FederatedMetricList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FederatedMetric `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FederatedMetric{}, &FederatedMetricList{})
}

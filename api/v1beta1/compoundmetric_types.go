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
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type GroupVersionResource struct {
	Group    string `json:"group,omitempty"`
	Version  string `json:"version,omitempty"`
	Resource string `json:"resource,omitempty"`
}

func (gvr *GroupVersionResource) String() string {
	return strings.Join([]string{gvr.Group, "/", gvr.Version, ", Resource=", gvr.Resource}, "")
}

type Projection struct {
	// Define the name of the field that should be extracted
	Name string `json:"name,omitempty"`

	// Define the path to the field that should be extracted
	FieldPath string `json:"fieldPath,omitempty"`
}

type Dimension struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// CompoundMetricSpec defines the desired state of CompoundMetric
type CompoundMetricSpec struct {
	Name string `json:"name,omitempty"`

	// +optional
	Description string `json:"description,omitempty"`

	// +kubebuilder:validation:Required
	Target GroupVersionResource `json:"target,omitempty"`

	// +optional
	Selectors Selectors `json:"selectors,inline"`

	Projections []Projection `json:"projections,omitempty"`

	// Define in what interval the query should be recorded (in minutes) # min: 1
	// +optional
	// +kubebuilder:default:=720
	// +kubebuilder:validation:Minimum=1
	Frequency int `json:"frequency,omitempty"`

	ClusterAccessFacade `json:",inline"`
}

// CompoundMetricStatus defines the observed state of CompoundMetric
type CompoundMetricStatus struct {

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
// +kubebuilder:printcolumn:name="OBSERVED",type="date",JSONPath=".status.observation.timestamp"

// CompoundMetric is the Schema for the compoundmetrics API
type CompoundMetric struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CompoundMetricSpec   `json:"spec,omitempty"`
	Status CompoundMetricStatus `json:"status,omitempty"`
}

func (r *CompoundMetric) SetConditions(conditions ...metav1.Condition) {
	for _, c := range conditions {
		meta.SetStatusCondition(&r.Status.Conditions, c)
	}
}

// +kubebuilder:object:root=true

// CompoundMetricList contains a list of CompoundMetric
type CompoundMetricList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CompoundMetric `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CompoundMetric{}, &CompoundMetricList{})
}

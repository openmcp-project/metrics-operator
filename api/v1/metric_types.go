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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MetricSpec defines the desired state of Metric
type MetricSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of Metric. Edit metric_types.go to remove/update
	Foo  string `json:"foo,omitempty"`
	Name string `json:"name,omitempty"`

	// +optional
	Kind string `json:"kind,omitempty"`
	// +optional
	LabelSelector string `json:"labelSelector,omitempty"`
	// +optional
	FieldSelector string `json:"fieldSelector,omitempty"`
}

// MetricStatus defines the observed state of Metric
type MetricStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Metric is the Schema for the metrics API
type Metric struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MetricSpec   `json:"spec,omitempty"`
	Status MetricStatus `json:"status,omitempty"`
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

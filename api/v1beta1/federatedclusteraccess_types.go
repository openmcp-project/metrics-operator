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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FederateCAFacade defines the desired state of FederatedClusterAccess
type FederateCAFacade struct {
	FederatedCARef FederateCARef `json:"federateCaRef,omitempty"`
}

// FederateCARef is a reference to a FederateCA
type FederateCARef struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// FederatedClusterAccessSpec defines the desired state of FederatedClusterAccess
type FederatedClusterAccessSpec struct {
	// Define the target resources that should be monitored
	Target GroupVersionResource `json:"target,omitempty"`

	// Field that contains the kubeconfig to access the target cluster. Use dot notation to access nested fields.
	KubeConfigPath string `json:"kubeConfigPath,omitempty"`

	// TODO: add label and field selectors

}

// GroupVersionResource defines the target resource
type GroupVersionResource struct {
	Group    string `json:"group,omitempty"`
	Version  string `json:"version,omitempty"`
	Resource string `json:"resource,omitempty"`
}

func (gvr *GroupVersionResource) String() string {
	return strings.Join([]string{gvr.Group, "/", gvr.Version, ", Resource=", gvr.Resource}, "")
}

// FederatedClusterAccessStatus defines the observed state of FederatedClusterAccess
type FederatedClusterAccessStatus struct {
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// FederatedClusterAccess is the Schema for the federatedclusteraccesses API
type FederatedClusterAccess struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FederatedClusterAccessSpec   `json:"spec,omitempty"`
	Status FederatedClusterAccessStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FederatedClusterAccessList contains a list of FederatedClusterAccess
type FederatedClusterAccessList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FederatedClusterAccess `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FederatedClusterAccess{}, &FederatedClusterAccessList{})
}

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

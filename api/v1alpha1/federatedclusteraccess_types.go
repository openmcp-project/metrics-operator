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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FederateClusterAccessRef is a reference to a FederateCA
type FederateClusterAccessRef struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// FederatedClusterAccessSpec defines the desired state of FederatedClusterAccess
// +kubebuilder:validation:XValidation:rule="(has(self.kubeConfigPath) && size(self.kubeConfigPath) > 0) != (has(self.secretRefPath) && size(self.secretRefPath) > 0)",message="exactly one of kubeConfigPath or secretRefPath must be set"
type FederatedClusterAccessSpec struct {
	// Define the target resources that should be monitored
	Target GroupVersionKind `json:"target,omitempty"`

	// Define labels of your object to adapt filters of the query
	// +optional
	LabelSelector string `json:"labelSelector,omitempty"`

	// Define fields of your object to adapt filters of the query
	// +optional
	FieldSelector string `json:"fieldSelector,omitempty"`

	// Restricts the scope of the target resource to a specific namespace
	// Only applicable for namespaced resources
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Field that contains the kubeconfig to access the target cluster. Use dot notation to access nested fields.
	// The field can be of type string or object.
	// Either KubeConfigPath or SecretRefPath must be set.
	// +optional
	KubeConfigPath string `json:"kubeConfigPath,omitempty"`

	// Field that contains the secret reference to access the target cluster. Use dot notation to access nested fields.
	// The field needs to be of type SecretRef and contain the reference to the secret that holds the kubeconfig (name, namespace, key).
	// If namespace is omitted, the namespace of target object will be used as default.
	// If key is omitted, "kubeconfig" will be used as default.
	// Either KubeConfigPath or SecretRefPath must be set.
	// +optional
	SecretRefPath string `json:"secretRefPath,omitempty"`
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

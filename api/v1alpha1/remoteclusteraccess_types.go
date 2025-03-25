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

// RemoteClusterAccessFacade is a facade to reference a RemoteClusterAccess type
type RemoteClusterAccessFacade struct {
	// Reference to the RemoteClusterAccess type that either reference a kubeconfig or a service account and cluster secret for remote access
	// +optional
	RemoteClusterAccessRef *RemoteClusterAccessRef `json:"remoteClusterAccessRef,omitempty"`
}

// RemoteClusterAccessRef is to be used by other types to reference a RemoteClusterAccess type
type RemoteClusterAccessRef struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// KubeConfigSecretRef is a reference to a secret that contains a kubeconfig using a specific key
type KubeConfigSecretRef struct {
	// Name is the name of the secret
	Name string `json:"name,omitempty"`
	// Namespace is the namespace of the secret
	Namespace string `json:"namespace,omitempty"`
	// Key is the key in the secret to use
	Key string `json:"key,omitempty"`
}

// RemoteClusterAccessSpec defines the desired state of RemoteClusterAccess
type RemoteClusterAccessSpec struct {

	// Reference to the secret that contains the kubeconfig to access an external cluster other than the one the operator is running in
	// +optional
	KubeConfigSecretRef *KubeConfigSecretRef `json:"kubeConfigSecretRef,omitempty"`

	// +optional
	ClusterAccessConfig *ClusterAccessConfig `json:"remoteClusterConfig,omitempty"`
}

// ClusterAccessConfig defines the configuration to access a remote cluster
type ClusterAccessConfig struct {
	ServiceAccountName      string `json:"serviceAccountName,omitempty"`
	ServiceAccountNamespace string `json:"serviceAccountNamespace,omitempty"`

	ClusterSecretRef RemoteClusterSecretRef `json:"clusterSecretRef,omitempty"`
}

// RemoteClusterSecretRef is a reference to a secret that contains host, audience, and caData to a remote cluster
type RemoteClusterSecretRef struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// RemoteClusterAccessStatus defines the observed state of RemoteClusterAccess
type RemoteClusterAccessStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// RemoteClusterAccess is the Schema for the remoteclusteraccesses API
type RemoteClusterAccess struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteClusterAccessSpec   `json:"spec,omitempty"`
	Status RemoteClusterAccessStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RemoteClusterAccessList contains a list of RemoteClusterAccess
type RemoteClusterAccessList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RemoteClusterAccess `json:"items"`
}

func init() {
	SchemeBuilder.Register(&RemoteClusterAccess{}, &RemoteClusterAccessList{})
}

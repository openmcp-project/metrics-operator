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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Connection defines the connection details for the DataSink
type Connection struct {
	// Endpoint specifies the target endpoint URL
	Endpoint string `json:"endpoint"`
	// Protocol specifies the communication protocol
	// +kubebuilder:validation:Enum=http;grpc
	Protocol string `json:"protocol"`
	// InsecureSkipVerify controls whether to skip TLS certificate verification
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// APIKeyAuthentication defines API key authentication configuration
type APIKeyAuthentication struct {
	// SecretKeyRef references a key in a Kubernetes Secret containing the API key
	SecretKeyRef corev1.SecretKeySelector `json:"secretKeyRef"`
}

// Authentication defines authentication mechanisms for the DataSink
type Authentication struct {
	// APIKey specifies API key authentication configuration
	// +optional
	APIKey *APIKeyAuthentication `json:"apiKey,omitempty"`
}

// DataSinkSpec defines the desired state of DataSink
type DataSinkSpec struct {
	// Connection specifies the connection details for the data sink
	Connection Connection `json:"connection"`
	// Authentication specifies the authentication configuration
	// +optional
	Authentication *Authentication `json:"authentication,omitempty"`
}

// DataSinkStatus defines the observed state of DataSink
type DataSinkStatus struct {
	// Conditions represent the latest available observations of an object's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// DataSink is the Schema for the datasinks API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ENDPOINT",type="string",JSONPath=".spec.connection.endpoint"
// +kubebuilder:printcolumn:name="PROTOCOL",type="string",JSONPath=".spec.connection.protocol"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
type DataSink struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataSinkSpec   `json:"spec,omitempty"`
	Status DataSinkStatus `json:"status,omitempty"`
}

// DataSinkList contains a list of DataSink
// +kubebuilder:object:root=true
type DataSinkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataSink `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataSink{}, &DataSinkList{})
}

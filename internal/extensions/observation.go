package extensions

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Observation is an interface that represents an observation
type Observation interface {
	GetTimestamp() metav1.Time
	GetValue() string
}

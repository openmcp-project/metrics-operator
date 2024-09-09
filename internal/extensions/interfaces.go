package extensions

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ObservationImpl interface {
	GetTimestamp() metav1.Time
	GetValue() string
}

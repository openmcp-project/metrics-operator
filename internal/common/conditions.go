package common

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/SAP/metrics-operator/api/v1alpha1"
)

// Creating returns a condition that indicates the resource being monitored is currently being created
func Creating() metav1.Condition {
	return metav1.Condition{
		Type:               v1alpha1.TypeCreating,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             v1alpha1.ReasonMonitoringActive,
	}
}

// Available returns a condition that indicates the resource being monitored is currently available
func Available(message string) metav1.Condition {
	return metav1.Condition{
		Type:               v1alpha1.TypeAvailable,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             v1alpha1.ReasonMonitoringActive,
		Message:            message,
	}
}

// Updated returns a condition that indicates the metric recently has been updated
func Updated() metav1.Condition {
	return metav1.Condition{
		Type:               v1alpha1.TypeUpdated,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             v1alpha1.ReasonMetricsUpdated,
	}
}

// Error returns a condition that indicates a unspecified error has occurred
func Error(message string) metav1.Condition {
	return metav1.Condition{
		Type:               v1alpha1.TypeError,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             v1alpha1.ReasonErrorDetected,
		Message:            message,
	}
}

// Unavailable returns a condition that indicates the resource being monitored is currently unavailable
// e.g. does the resource with the correct filter exist in the cluster?
func Unavailable(message string) metav1.Condition {
	return metav1.Condition{
		Type:               v1alpha1.TypeUnavailable,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             v1alpha1.ReasonInactive,
		Message:            message,
	}
}

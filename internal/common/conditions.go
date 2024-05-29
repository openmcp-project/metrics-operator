package common

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

const (
	TypeAvailable   = "Available"
	TypeCreating    = "Creating"
	TypeUpdated     = "Updated"
	TypeUnavailable = "Unavailable"
	TypeError       = "Error"
)

func Creating() metav1.Condition {
	return metav1.Condition{
		Type:               TypeCreating,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "MetricsCreating",
	}
}

// Available returns a condition that indicates the resource being monitored is currently available
func Available(message string) metav1.Condition {
	return metav1.Condition{
		Type:               TypeAvailable,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "MonitoringActive",
		Message:            message,
	}
}

// Updated returns a condition that indicates the metric recently has been updated
func Updated() metav1.Condition {
	return metav1.Condition{
		Type:               TypeUpdated,
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "MetricsUpdated",
	}
}

// Error returns a condition that indicates a unspecified error has occurred
func Error(message string) metav1.Condition {
	return metav1.Condition{
		Type:               TypeError,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             "ErrorDetected",
		Message:            message,
	}
}

// Unavailable returns a condition that indicates the resource being monitored is currently unavailable
// e.g. does the resource with the correct filter exist in the cluster?
func Unavailable(message string) metav1.Condition {
	return metav1.Condition{
		Type:               TypeUnavailable,
		Status:             metav1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             "MonitoringInactive",
		Message:            message,
	}
}

package v1alpha1

const (
	// ReasonMonitoringActive is used to indicate that the metric is currently monitoring the resource
	ReasonMonitoringActive = "MonitoringActive"

	// ReasonSendMetricFailed is used to indicate that the metric failed to send the metric value to the data sink
	ReasonSendMetricFailed = "SendMetricFailed"

	// ReasonMetricsUpdated is used to indicate that the metric has been updated
	ReasonMetricsUpdated = "MetricsUpdated"

	// ReasonErrorDetected is used to indicate that an error has been detected
	ReasonErrorDetected = "ErrorDetected"

	// ReasonInactive is used to indicate that the resource being monitored is currently unavailable
	ReasonInactive = "MonitoringInactive"

	// ReasonMetricsCreating is used to indicate that the metric is currently being crevated
	ReasonMetricsCreating = "MetricsCreating"

	// TypeAvailable is a generic condition type that indicates the resource being monitored is currently available
	TypeAvailable = "Available"

	// TypeCreating is a generic condition type that indicates the resource being monitored is currently being created
	TypeCreating = "Creating"

	// TypeUpdated is a generic condition type that indicates the metric has been updated
	TypeUpdated = "Updated"

	// TypeUnavailable is a generic condition type that indicates the resource being monitored is currently unavailable
	TypeUnavailable = "Unavailable"

	// TypeError is a generic condition type that indicates an error has occurred
	TypeError = "Error"

	// TypeReady is a condition type that indicates the resource is ready
	TypeReady = "Ready"

	// StatusStringTrue represents the True status string.
	StatusStringTrue string = "True"
	// StatusStringFalse represents the False status string.
	StatusStringFalse string = "False"
	// StatusStringUnknown represents the Unknown status string.
	StatusStringUnknown string = "Unknown"
)

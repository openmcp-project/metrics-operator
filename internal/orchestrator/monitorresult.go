package orchestrator

import (
	insight "github.com/SAP/metrics-operator/api/v1alpha1"
	"github.com/SAP/metrics-operator/internal/extensions"
)

// MonitorResult is used to monitor the metric
type MonitorResult struct {
	Phase   insight.PhaseType
	Reason  string
	Message string
	Error   error

	Observation extensions.Observation
}

package orchestrator

import (
	insight "github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"github.com/openmcp-project/metrics-operator/internal/extensions"
)

// MonitorResult is used to monitor the metric
type MonitorResult struct {
	Phase   insight.PhaseType
	Reason  string
	Message string
	Error   error

	Observation extensions.Observation
}

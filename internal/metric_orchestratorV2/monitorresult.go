package metric_orchestratorV2

import (
	insight "github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1alpha1"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/internal/extensions"
)

type MonitorResult struct {
	Phase   insight.PhaseType
	Reason  string
	Message string
	Error   error

	Observation extensions.ObservationImpl
}

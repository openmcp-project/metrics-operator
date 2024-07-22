package metric_orchestratorV2

import insight "github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1alpha1"

type MonitorResult struct {
	Phase   insight.PhaseType
	Reason  string
	Message string
	Error   error
}

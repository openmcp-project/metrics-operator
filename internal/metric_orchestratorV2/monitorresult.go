package metric_orchestratorV2

import businessv1 "github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1"

type MonitorResult struct {
	Phase   businessv1.PhaseType
	Reason  string
	Message string
	Error   error
}

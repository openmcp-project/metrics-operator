package controller

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/metrics-operator/internal/common"
	"github.com/openmcp-project/metrics-operator/internal/orchestrator"
)

// InsightReconciler is an interface for the reconciler of Insight objects
type InsightReconciler interface {
	getClient() client.Client
	getRestConfig() *rest.Config
}

func monitoringFailureCondition(result orchestrator.MonitorResult) metav1.Condition {
	message := result.Message
	if result.Error != nil && message != result.Error.Error() {
		message = fmt.Sprintf("%s: %v", message, result.Error)
	}
	reason := result.Reason
	if reason == "" {
		reason = "MonitoringFailed"
	}
	return common.ReadyFalse(reason, message)
}

package controller

import (
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// InsightReconciler is an interface for the reconciler of Insight objects
type InsightReconciler interface {
	getClient() client.Client
	getRestConfig() *rest.Config
}

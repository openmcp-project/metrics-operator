package controller

import (
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type InsightReconciler interface {
	GetClient() client.Client
	GetRestConfig() *rest.Config
}

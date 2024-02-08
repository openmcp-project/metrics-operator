package metricorchestrator

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ctrl "sigs.k8s.io/controller-runtime"

	businessv1 "github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1"
	handler "github.tools.sap/cloud-orchestration/co-metrics-operator/internal/metric-orchestrator/handler"
)

// This is used to have a singel point of entry for all metric types
//
// This lets you call which metric should be handled from a single point of entry
type MetricOrchestrator struct {
	managedHandler handler.ManagedMetricHandler
	genericHandler handler.GenericMetricHandler
}

// Create a new Metric Orchestrator for your Reconciler
// Params are needed to access the cluster in the conext of your reconciler
func NewMetricOrchestrator(ctx context.Context, req ctrl.Request, client client.Client, cfg *rest.Config) (MetricOrchestrator, error) {

	dClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return MetricOrchestrator{}, err
	}

	managed, err := handler.NewMangedMetricHandler(ctx, req, client, dClient)
	if err != nil {
		return MetricOrchestrator{}, err
	}

	generic, err := handler.NewGenericMetricHandler(ctx, req, client, dClient)
	if err != nil {
		return MetricOrchestrator{}, err
	}

	orchestrator := MetricOrchestrator{
		managedHandler: managed,
		genericHandler: generic,
	}

	return orchestrator, nil
}

// Handle the type ManagedMetric
// This function will take care of creating, sending and pulling the information of your defined metric to dynatrace
func (m *MetricOrchestrator) OrchestrateManagedMetric() (int, businessv1.ActivationType, error) {
	frequency, status, err := m.managedHandler.HandleManagedMetric()
	if err != nil {
		return -1, status, err
	}
	fmt.Printf("%s	INFO	Managed Metric Handler entered\n", time.Now().UTC().Format("2006-01-02T15:04:05+01:00"))
	return frequency, status, nil
}

// Handle the type GenericMetric
// This function will take care of creating, sending and pulling the information of your defined metric to dynatrace
func (m *MetricOrchestrator) OrchestrateGenericMetric() (int, businessv1.ActivationType, error) {
	frequency, status, err := m.genericHandler.HandleGenericMetric()
	if err != nil {
		return -1, status, err
	}

	return frequency, status, nil
}

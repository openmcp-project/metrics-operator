package orchestrator

import (
	"context"

	"k8s.io/client-go/rest"
	rcli "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/SAP/metrics-operator/api/v1alpha1"
	"github.com/SAP/metrics-operator/api/v1beta1"
	"github.com/SAP/metrics-operator/internal/client"

	// "github.com/SAP/metrics-operator/internal/clientlite" // Removed
	"github.com/SAP/metrics-operator/internal/clientoptl"
	"github.com/SAP/metrics-operator/internal/common"
)

// MetricHandler is used to monitor the metric
type MetricHandler interface {
	Monitor(ctx context.Context) (MonitorResult, error)
}

// Orchestrator is used to create a new handler
type Orchestrator struct {
	Handler MetricHandler

	credentials common.DataSinkCredentials

	queryConfig QueryConfig
}

// QueryConfig holds the configuration for the query client to query resources in a K8S cluster, may be internal or external cluster.
type QueryConfig struct {
	Client      rcli.Client
	RestConfig  rest.Config
	ClusterName *string
}

// NewOrchestrator creates a new Orchestrator
func NewOrchestrator(creds common.DataSinkCredentials, qConfig QueryConfig) *Orchestrator {
	return &Orchestrator{credentials: creds, queryConfig: qConfig}
}

// WithGeneric creates a new Orchestrator with a Generic handler
func (o *Orchestrator) WithGeneric(metric v1.Metric) (*Orchestrator, error) {
	dtClient := client.NewClient(o.credentials.Host, o.credentials.Path, o.credentials.Token)
	metricMetadata := client.NewMetricMetadata(metric.Spec.Name, metric.Spec.Name, metric.Spec.Description)

	var err error
	o.Handler, err = NewGenericHandler(metric, metricMetadata, o.queryConfig, dtClient)
	return o, err
}

// WithSingle creates a new Orchestrator with a SingleMetric handler
func (o *Orchestrator) WithSingle(metric v1beta1.SingleMetric, gaugeMetric *clientoptl.Metric) (*Orchestrator, error) { // Added gaugeMetric parameter
	// dtClient creation removed, as it's handled by the controller

	var err error
	// Pass gaugeMetric instead of dtClient
	o.Handler, err = NewSingleHandler(metric, o.queryConfig, gaugeMetric)
	return o, err
}

// WithManaged creates a new Orchestrator with a ManagedMetric handler
func (o *Orchestrator) WithManaged(managed v1.ManagedMetric) (*Orchestrator, error) {
	dtClient := client.NewClient(o.credentials.Host, o.credentials.Path, o.credentials.Token)
	metricMetadata := client.NewMetricMetadata(managed.Spec.Name, managed.Spec.Name, managed.Spec.Description)

	var err error
	o.Handler, err = NewManagedHandler(managed, metricMetadata, o.queryConfig, dtClient)
	return o, err
}

// WithCompound creates a new Orchestrator with a CompoundMetric handler
func (o *Orchestrator) WithCompound(metric v1beta1.CompoundMetric, gaugeMetric *clientoptl.Metric) (*Orchestrator, error) { // Added gaugeMetric parameter
	// dtClient creation removed, as it's handled by the controller

	var err error
	// Pass gaugeMetric instead of dtClient
	o.Handler, err = NewCompoundHandler(metric, o.queryConfig, gaugeMetric)
	return o, err
}

// WithFederated creates a new Orchestrator with a FederatedMetric handler
func (o *Orchestrator) WithFederated(metric v1beta1.FederatedMetric, gaugeMetric *clientoptl.Metric) (*Orchestrator, error) {
	var err error
	o.Handler, err = NewFederatedHandler(metric, o.queryConfig, gaugeMetric)
	return o, err
}

// WithFederatedManaged creates a new Orchestrator with a FederatedManagedMetric handler
func (o *Orchestrator) WithFederatedManaged(metric v1beta1.FederatedManagedMetric, gaugeMetric *clientoptl.Metric) (*Orchestrator, error) {
	var err error
	o.Handler, err = NewFederatedManagedHandler(metric, o.queryConfig, gaugeMetric)
	return o, err
}

package orchestrator

import (
	"context"

	"k8s.io/client-go/rest"
	rcli "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"github.com/openmcp-project/metrics-operator/internal/clientoptl"
	"github.com/openmcp-project/metrics-operator/internal/common"
)

const (
	// KIND Constant for k8s resource fields
	KIND string = "kind"

	// GROUP Constant for k8s resource fields
	GROUP string = "group"

	// VERSION Constant for k8s resource fields
	VERSION string = "version"

	// CLUSTER Constant for k8s resource fields
	CLUSTER string = "cluster"

	// RESOURCE Constant for k8s resource fields
	RESOURCE string = "resource"

	// APIVERSION Constant for k8s resource fields
	APIVERSION string = "apiVersion"

	// NAME Constant for k8s resource fields
	CR_NAME string = "crName"
)

// GenericHandler is used to monitor the metric
type GenericHandler interface {
	Monitor(ctx context.Context) (MonitorResult, error)
}

// Orchestrator is used to create a new handler
type Orchestrator struct {
	Handler GenericHandler

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

// WithManaged creates a new Orchestrator with a ManagedMetric handler
func (o *Orchestrator) WithManaged(managed v1alpha1.ManagedMetric, gaugeMetric *clientoptl.Metric) (*Orchestrator, error) {
	var err error
	o.Handler, err = NewManagedHandler(managed, o.queryConfig, gaugeMetric)
	return o, err
}

// WithMetric creates a new Orchestrator with a Metric handler
func (o *Orchestrator) WithMetric(metric v1alpha1.Metric, gaugeMetric *clientoptl.Metric) (*Orchestrator, error) { // Added gaugeMetric parameter
	// dtClient creation removed, as it's handled by the controller

	var err error
	// Pass gaugeMetric instead of dtClient
	o.Handler, err = NewMetricHandler(metric, o.queryConfig, gaugeMetric)
	return o, err
}

// WithFederated creates a new Orchestrator with a FederatedMetric handler
func (o *Orchestrator) WithFederated(metric v1alpha1.FederatedMetric, gaugeMetric *clientoptl.Metric) (*Orchestrator, error) {
	var err error
	o.Handler, err = NewFederatedHandler(metric, o.queryConfig, gaugeMetric)
	return o, err
}

// WithFederatedManaged creates a new Orchestrator with a FederatedManagedMetric handler
func (o *Orchestrator) WithFederatedManaged(metric v1alpha1.FederatedManagedMetric, gaugeMetric *clientoptl.Metric) (*Orchestrator, error) {
	var err error
	o.Handler, err = NewFederatedManagedHandler(metric, o.queryConfig, gaugeMetric)
	return o, err
}

package metric_orchestratorV2

import (
	v1 "github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/internal/client"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/internal/common"
	"k8s.io/client-go/rest"
	rcli "sigs.k8s.io/controller-runtime/pkg/client"
)

type MetricHandler interface {
	Monitor() (MonitorResult, error)
}

type Orchestrator struct {
	Handler MetricHandler

	restConfig  *rest.Config
	credentials common.DataSinkCredentials
	client      rcli.Client
}

func NewOrchestrator(config *rest.Config, creds common.DataSinkCredentials, runCli rcli.Client) *Orchestrator {
	return &Orchestrator{credentials: creds, restConfig: config, client: runCli}
}

func (o *Orchestrator) WithGeneric(metric v1.Metric) (*Orchestrator, error) {
	dtClient := client.NewClient(o.credentials.Host, o.credentials.Path, o.credentials.Token)
	metricMetadata := client.NewMetricMetadata(metric.ObjectMeta.Name, metric.Spec.Name, metric.Spec.Description)

	var err error
	o.Handler, err = NewGenericHandler(metric, metricMetadata, o.restConfig, dtClient)
	return o, err
}

func (o *Orchestrator) WithManaged(managed v1.ManagedMetric) (*Orchestrator, error) {
	dtClient := client.NewClient(o.credentials.Host, o.credentials.Path, o.credentials.Token)
	metricMetadata := client.NewMetricMetadata(managed.ObjectMeta.Name, managed.Spec.Name, managed.Spec.Description)

	var err error
	o.Handler, err = NewManagedHandler(managed, metricMetadata, o.restConfig, o.client, dtClient)
	return o, err
}

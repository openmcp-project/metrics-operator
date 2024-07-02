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

	credentials common.DataSinkCredentials

	queryConfig QueryConfig
}

// QueryConfig holds the configuration for the query client to query resources in a K8S cluster, may be internal or external cluster.
type QueryConfig struct {
	Client      rcli.Client
	RestConfig  rest.Config
	ClusterName *string
}

func NewOrchestrator(creds common.DataSinkCredentials, qConfig QueryConfig) *Orchestrator {
	return &Orchestrator{credentials: creds, queryConfig: qConfig}
}

func (o *Orchestrator) WithGeneric(metric v1.Metric) (*Orchestrator, error) {
	dtClient := client.NewClient(o.credentials.Host, o.credentials.Path, o.credentials.Token)
	metricMetadata := client.NewMetricMetadata(metric.Spec.Name, metric.Spec.Name, metric.Spec.Description)

	var err error
	o.Handler, err = NewGenericHandler(metric, metricMetadata, o.queryConfig, dtClient)
	return o, err
}

func (o *Orchestrator) WithManaged(managed v1.ManagedMetric) (*Orchestrator, error) {
	dtClient := client.NewClient(o.credentials.Host, o.credentials.Path, o.credentials.Token)
	metricMetadata := client.NewMetricMetadata(managed.Spec.Name, managed.Spec.Name, managed.Spec.Description)

	var err error
	o.Handler, err = NewManagedHandler(managed, metricMetadata, o.queryConfig, dtClient)
	return o, err
}

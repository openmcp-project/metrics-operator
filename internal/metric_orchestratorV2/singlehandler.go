package metric_orchestratorV2

import (
	"context"
	"fmt"
	"strconv"

	"github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1alpha1"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1beta1"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/internal/clientlite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

type SingleHandler struct {
	dCli        dynamic.Interface
	discoClient discovery.DiscoveryInterface

	metric v1beta1.SingleMetric

	dtClient    *clientlite.MetricClient
	clusterName *string
}

func (h *SingleHandler) Monitor() (MonitorResult, error) {

	mrTotal := h.createGvkBaseMetric()

	if h.clusterName != nil {
		mrTotal.AddDimension(CLUSTER, *h.clusterName)
	}

	result := MonitorResult{}

	list, err := h.getResources()
	if err != nil {
		return MonitorResult{}, fmt.Errorf("could not retrieve target resource(s) %w", err)
	}

	primaryCount := len(list.Items)
	mrTotal.SetGaugeValue(float64(primaryCount))

	errMetric := h.dtClient.SendMetrics(mrTotal)

	if errMetric != nil {
		result.Error = err
		result.Phase = v1alpha1.PhaseFailed
		result.Reason = "SendMetricFailed"
		result.Message = fmt.Sprintf("failed to send metric value to data sink. %s", errMetric.Error())
	} else {
		result.Phase = v1alpha1.PhaseActive
		result.Reason = "MonitoringActive"
		result.Message = fmt.Sprintf("metric is monitoring resource '%s'", h.metric.GvkToString())
		result.Observation = &v1beta1.MetricObservation{Timestamp: metav1.Now(), LatestValue: strconv.Itoa(primaryCount)}
	}
	return result, nil

}

func (h *SingleHandler) getResources() (*unstructured.UnstructuredList, error) {
	var options = metav1.ListOptions{}
	// if not defined in the metric, the list options need to be empty to get resources based on GVR only
	if h.metric.Spec.Selectors.LabelSelector != "" && h.metric.Spec.Selectors.FieldSelector != "" {
		options = metav1.ListOptions{LabelSelector: h.metric.Spec.Selectors.LabelSelector, FieldSelector: h.metric.Spec.Selectors.FieldSelector}
	}

	gvk := schema.GroupVersionKind{Group: h.metric.Spec.Target.Group, Version: h.metric.Spec.Target.Version, Kind: h.metric.Spec.Target.Kind}
	gvr, err := getGVRfromGVK(gvk, h.discoClient)

	if err != nil {
		return nil, fmt.Errorf("could not find GVR from GVK with filter '%s'. %w", h.metric.GvkToString(), err)
	}

	list, err := h.dCli.Resource(gvr).List(context.Background(), options)

	if err != nil {
		return nil, fmt.Errorf("could not find any matching resources for metric with filter '%s'. %w", h.metric.GvkToString(), err)
	}

	return list, nil
}

func (h *SingleHandler) createGvkBaseMetric() *clientlite.Metric {
	return clientlite.NewMetric(h.metric.Name).
		AddDimension(GROUP, h.metric.Spec.Target.Group).
		AddDimension(VERSION, h.metric.Spec.Target.Version).
		AddDimension(KIND, h.metric.Spec.Target.Kind)
}

func NewSingleHandler(metric v1beta1.SingleMetric, qc QueryConfig, dtClient *clientlite.MetricClient) (*SingleHandler, error) {
	dynamicClient, errCli := dynamic.NewForConfig(&qc.RestConfig)
	if errCli != nil {
		return nil, errCli
	}

	disco, errDisco := discovery.NewDiscoveryClientForConfig(&qc.RestConfig)
	if errDisco != nil {
		return nil, errDisco
	}

	var handler = &SingleHandler{
		metric:      metric,
		dCli:        dynamicClient,
		discoClient: disco,
		dtClient:    dtClient,
		clusterName: qc.ClusterName,
	}

	return handler, nil
}

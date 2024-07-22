package metric_orchestratorV2

import (
	"context"
	"fmt"

	"github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1alpha1"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/internal/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

const (
	KIND    string = "kind"
	GROUP   string = "group"
	VERSION string = "version"
	CLUSTER string = "cluster"
)

type GenericHandler struct {
	dCli        dynamic.Interface
	discoClient discovery.DiscoveryInterface

	metric     v1alpha1.Metric
	metricMeta client.MetricMetadata

	dtClient    client.DynatraceClient
	clusterName *string
}

func NewGenericHandler(metric v1alpha1.Metric, metricMeta client.MetricMetadata, qc QueryConfig, dtClient client.DynatraceClient) (*GenericHandler, error) {
	dynamicClient, errCli := dynamic.NewForConfig(&qc.RestConfig)
	if errCli != nil {
		return nil, fmt.Errorf("could not create dynamic client: %w", errCli)
	}

	disco, errDisco := discovery.NewDiscoveryClientForConfig(&qc.RestConfig)
	if errDisco != nil {
		return nil, fmt.Errorf("could not create discovery client: %w", errDisco)
	}

	var handler = &GenericHandler{
		metric:      metric,
		metricMeta:  metricMeta,
		dCli:        dynamicClient,
		discoClient: disco,
		dtClient:    dtClient,
		clusterName: qc.ClusterName,
	}

	return handler, nil
}

func (h *GenericHandler) sendMetricValue() error {

	count, err := h.getResourceCount(h.dCli)
	if err != nil {
		return err
	}

	h.metricMeta.AddDatapoint(float64(count))
	_, err = h.dtClient.SendMetric(h.metricMeta)

	// if no err, returns nil...duh!
	return err
}

func (h *GenericHandler) Monitor() (MonitorResult, error) {

	kindDimErr := h.metricMeta.AddDimension(KIND, h.metric.Spec.Kind)
	if kindDimErr != nil {
		return MonitorResult{}, fmt.Errorf("could not initialize '"+KIND+"' dimensions: %w", kindDimErr)
	}
	groupDimErr := h.metricMeta.AddDimension(GROUP, h.metric.Spec.Group)
	if groupDimErr != nil {
		return MonitorResult{}, fmt.Errorf("could not initialize '"+GROUP+"' dimensions: %w", groupDimErr)
	}
	versionDimErr := h.metricMeta.AddDimension(VERSION, h.metric.Spec.Version)
	if versionDimErr != nil {
		return MonitorResult{}, fmt.Errorf("could not initialize '"+VERSION+"' dimensions: %w", versionDimErr)
	}

	if h.clusterName != nil {
		clusterDimErr := h.metricMeta.AddDimension(CLUSTER, *h.clusterName)
		if clusterDimErr != nil {
			return MonitorResult{}, fmt.Errorf("could not initialize '"+CLUSTER+"' dimensions: %w", clusterDimErr)
		}
	}

	result := MonitorResult{}
	err := h.sendMetricValue()

	if err != nil {
		result.Error = err
		result.Phase = v1alpha1.PhaseFailed
		result.Reason = "SendMetricFailed"
		result.Message = fmt.Sprintf("failed to send metric value to data sink. %s", err.Error())
	} else {
		result.Phase = v1alpha1.PhaseActive
		result.Reason = "MonitoringActive"
		result.Message = fmt.Sprintf("metric is monitoring resource '%s'", h.metric.GvkToString())
	}

	return result, nil
}

func (h *GenericHandler) getResourceCount(dCli dynamic.Interface) (int, error) {
	var options = metav1.ListOptions{}
	// if not defined in the metric, the list options need to be empty to get resources based on GVR only
	if h.metric.Spec.LabelSelector != "" && h.metric.Spec.FieldSelector != "" {
		options = metav1.ListOptions{LabelSelector: h.metric.Spec.LabelSelector, FieldSelector: h.metric.Spec.FieldSelector}
	}

	gvk := schema.GroupVersionKind{Group: h.metric.Spec.Group, Version: h.metric.Spec.Version, Kind: h.metric.Spec.Kind}
	gvr, err := getGVRfromGVK(gvk, h.discoClient)

	if err != nil {
		return 0, fmt.Errorf("could not find GVR from GVK with filter '%s'. %w", h.metric.GvkToString(), err)
	}

	list, err := dCli.Resource(gvr).List(context.Background(), options)

	if err != nil {
		return 0, fmt.Errorf("could not find any matching resources for metric with filter '%s'. %w", h.metric.GvkToString(), err)
	}

	return len(list.Items), nil
}

func getGVRfromGVK(gvk schema.GroupVersionKind, disco discovery.DiscoveryInterface) (schema.GroupVersionResource, error) {
	// TODO: this could be optimized later (e.g. by caching the discovery client)

	groupResources, err := disco.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		return schema.GroupVersionResource{}, err
	}

	for _, resource := range groupResources.APIResources {
		if resource.Kind == gvk.Kind {
			return schema.GroupVersionResource{
				Group:    gvk.Group,
				Version:  gvk.Version,
				Resource: resource.Name,
			}, nil
		}
	}

	return schema.GroupVersionResource{}, nil
}

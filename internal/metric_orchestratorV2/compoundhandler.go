package metric_orchestratorV2

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1alpha1"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1beta1"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/internal/clientlite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

type CompoundHandler struct {
	dCli        dynamic.Interface
	discoClient discovery.DiscoveryInterface

	metric v1beta1.CompoundMetric

	dtClient    *clientlite.MetricClient
	clusterName *string
}

func (h *CompoundHandler) Monitor() (MonitorResult, error) {

	mrTotal := h.createGvrBaseMetric()

	if h.clusterName != nil {
		mrTotal.AddDimension(CLUSTER, *h.clusterName)
	}

	result := MonitorResult{Observation: &v1beta1.MetricObservation{Timestamp: metav1.Now()}}

	list, err := h.getResources()
	if err != nil {
		return MonitorResult{}, fmt.Errorf("could not retrieve target resource(s) %w", err)
	}

	groups := h.extractProjectionGroupsFrom(list)

	var dimensions []v1beta1.Dimension

	var clMetrics []*clientlite.Metric
	clMetrics = append(clMetrics, mrTotal)

	for _, group := range groups {

		mrGroup := h.createGvrBaseMetric()
		mrGroup.SetGaugeValue(float64(len(group)))
		clMetrics = append(clMetrics, mrGroup)

		for _, pField := range group {
			if pField.error == nil {
				mrGroup.AddDimension(pField.name, pField.value)
				dimensions = append(dimensions, v1beta1.Dimension{Name: pField.name, Value: pField.value})
			}
		}

	}

	err = h.dtClient.SendMetrics(clMetrics...)

	if err != nil {
		result.Error = err
		result.Phase = v1alpha1.PhaseFailed
		result.Reason = "SendMetricFailed"
		result.Message = fmt.Sprintf("failed to send metric value to data sink. %s", err.Error())
	} else {
		result.Phase = v1alpha1.PhaseActive
		result.Reason = "MonitoringActive"
		result.Message = fmt.Sprintf("metric is monitoring resource '%s'", h.metric.Spec.Target.String())

		if dimensions != nil {
			result.Observation = &v1beta1.MetricObservation{Timestamp: metav1.Now(), Dimensions: []v1beta1.Dimension{{Name: dimensions[0].Name, Value: strconv.Itoa(len(list.Items))}}}
		}
	}

	return result, nil
}

type projectedField struct {
	name  string
	value string
	found bool
	error error
}

func (e *projectedField) GetID() string {
	return fmt.Sprintf("%s: %s", e.name, e.value)
}

func (h *CompoundHandler) extractProjectionGroupsFrom(list *unstructured.UnstructuredList) map[string][]projectedField {

	// note: for now we only allow one projection, so we can use the first one
	// the reason for this is that if we have multiple projections, we need to create a cartesian product of all projections
	// this is to be done at a later time

	var collection []projectedField

	for _, obj := range list.Items {

		projection := lo.FirstOr(h.metric.Spec.Projections, v1beta1.Projection{})

		if projection.Name != "" && projection.FieldPath != "" {
			name := projection.Name
			fieldPath := projection.FieldPath
			fields := strings.Split(fieldPath, ".")
			value, found, err := unstructured.NestedString(obj.Object, fields...)
			collection = append(collection, projectedField{name: name, value: value, found: found, error: err})
		}
	}

	// group by the extracted values for the dimension .e.g. device: iPhone, device: Android and count them later
	groups := lo.GroupBy(collection, func(field projectedField) string {
		return field.GetID()
	})

	return groups
}

func (h *CompoundHandler) createGvrBaseMetric() *clientlite.Metric {
	return clientlite.NewMetric(h.metric.Name).
		AddDimension(RESOURCE, h.metric.Spec.Target.Resource).
		AddDimension(GROUP, h.metric.Spec.Target.Group).
		AddDimension(VERSION, h.metric.Spec.Target.Version)
}

func (h *CompoundHandler) getResources() (*unstructured.UnstructuredList, error) {
	var options = metav1.ListOptions{}
	// if not defined in the metric, the list options need to be empty to get resources based on GVR only
	// Add label selector if present
	if h.metric.Spec.LabelSelector != "" {
		options.LabelSelector = h.metric.Spec.LabelSelector
	}

	// Add field selector if present
	if h.metric.Spec.FieldSelector != "" {
		options.FieldSelector = h.metric.Spec.FieldSelector
	}
	gvr := schema.GroupVersionResource{
		Group:    h.metric.Spec.Target.Group,
		Version:  h.metric.Spec.Target.Version,
		Resource: h.metric.Spec.Target.Resource,
	}
	list, err := h.dCli.Resource(gvr).List(context.Background(), options)

	if err != nil {
		return nil, fmt.Errorf("could not find any matching resources for metric set with filter '%s'. %w", gvr.String(), err)
	}

	return list, nil
}

func NewCompoundHandler(metric v1beta1.CompoundMetric, qc QueryConfig, dtClient *clientlite.MetricClient) (*CompoundHandler, error) {
	dynamicClient, errCli := dynamic.NewForConfig(&qc.RestConfig)
	if errCli != nil {
		return nil, errCli
	}

	disco, errDisco := discovery.NewDiscoveryClientForConfig(&qc.RestConfig)
	if errDisco != nil {
		return nil, errDisco
	}

	var handler = &CompoundHandler{
		metric:      metric,
		dCli:        dynamicClient,
		discoClient: disco,
		dtClient:    dtClient,
		clusterName: qc.ClusterName,
	}

	return handler, nil
}

package orchestrator

import (
	"context"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	"github.com/SAP/metrics-operator/api/v1alpha1"
	"github.com/SAP/metrics-operator/api/v1beta1"
	"github.com/SAP/metrics-operator/internal/clientoptl" // Added
)

// SingleHandler is used to monitor a single metric
type SingleHandler struct {
	dCli        dynamic.Interface
	discoClient discovery.DiscoveryInterface

	metric v1beta1.SingleMetric

	gaugeMetric *clientoptl.Metric // Changed from dtClient
	clusterName *string
}

// Monitor is used to monitor the metric
func (h *SingleHandler) Monitor(ctx context.Context) (MonitorResult, error) {

	// Metric creation and export are handled by the controller.
	// This handler focuses on fetching the value and recording it.

	result := MonitorResult{}

	list, errGet := h.getResources(ctx)
	if errGet != nil {
		result.Error = errGet
		result.Phase = v1alpha1.PhaseFailed
		result.Reason = "GetResourcesFailed"
		result.Message = fmt.Sprintf("failed to retrieve target resource(s): %s", errGet.Error())
		return result, nil // Return error state, but not the error itself to controller
	}

	primaryCount := len(list.Items)
	// Create DataPoint and record it
	dataPoint := clientoptl.NewDataPoint().SetValue(int64(primaryCount))

	// Add dimensions only if they have a non-empty value
	if h.metric.Spec.Target.Group != "" {
		dataPoint.AddDimension(GROUP, h.metric.Spec.Target.Group)
	}
	if h.metric.Spec.Target.Version != "" {
		dataPoint.AddDimension(VERSION, h.metric.Spec.Target.Version)
	}
	if h.metric.Spec.Target.Kind != "" {
		dataPoint.AddDimension(KIND, h.metric.Spec.Target.Kind)
	}
	if h.clusterName != nil && *h.clusterName != "" {
		dataPoint.AddDimension(CLUSTER, *h.clusterName)
	}

	errRecord := h.gaugeMetric.RecordMetrics(ctx, dataPoint)

	if errRecord != nil {
		result.Error = errRecord
		result.Phase = v1alpha1.PhaseFailed
		result.Reason = "RecordMetricFailed"
		result.Message = fmt.Sprintf("failed to record metric value: %s", errRecord.Error())
	} else {
		result.Phase = v1alpha1.PhaseActive
		result.Reason = "MonitoringActive"
		result.Message = fmt.Sprintf("metric value recorded for resource '%s'", h.metric.GvkToString())
		result.Observation = &v1beta1.MetricObservation{Timestamp: metav1.Now(), LatestValue: strconv.Itoa(primaryCount)}
	}

	// Return the result, error indicates failure in Monitor execution, not necessarily metric export failure (handled by controller)
	return result, nil

}

func (h *SingleHandler) getResources(ctx context.Context) (*unstructured.UnstructuredList, error) {
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

	gvk := schema.GroupVersionKind{Group: h.metric.Spec.Target.Group, Version: h.metric.Spec.Target.Version, Kind: h.metric.Spec.Target.Kind}
	gvr, err := getGVRfromGVK(gvk, h.discoClient)

	if err != nil {
		return nil, fmt.Errorf("could not find GVR from GVK with filter '%s'. %w", h.metric.GvkToString(), err)
	}

	list, err := h.dCli.Resource(gvr).List(ctx, options)

	if err != nil {
		return nil, fmt.Errorf("could not find any matching resources for metric with filter '%s'. %w", h.metric.GvkToString(), err)
	}

	return list, nil
}

// Removed createGvkBaseMetric as it's clientlite specific

// NewSingleHandler creates a new SingleHandler
func NewSingleHandler(metric v1beta1.SingleMetric, qc QueryConfig, gaugeMetric *clientoptl.Metric) (*SingleHandler, error) { // Changed dtClient to gaugeMetric
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
		gaugeMetric: gaugeMetric, // Changed dtClient to gaugeMetric
		clusterName: qc.ClusterName,
	}

	return handler, nil
}

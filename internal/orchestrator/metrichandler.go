package orchestrator

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	"github.com/SAP/metrics-operator/api/v1alpha1"
	"github.com/SAP/metrics-operator/internal/clientoptl"
)

// MetricHandler is used to monitor a metric
type MetricHandler struct {
	dCli        dynamic.Interface
	discoClient discovery.DiscoveryInterface

	metric v1alpha1.Metric

	gaugeMetric *clientoptl.Metric // Changed from dtClient
	clusterName *string
}

// Monitor is used to monitor the metric
//
//nolint:gocyclo
func (h *MetricHandler) Monitor(ctx context.Context) (MonitorResult, error) {

	// Metric creation and export are handled by the controller.
	// This handler focuses on fetching resources, grouping, and recording data points.
	result := MonitorResult{Observation: &v1alpha1.MetricObservation{Timestamp: metav1.Now()}}

	list, errGet := h.getResources(ctx)
	if errGet != nil {
		result.Error = errGet
		result.Phase = v1alpha1.PhaseFailed
		result.Reason = "GetResourcesFailed"
		result.Message = fmt.Sprintf("failed to retrieve target resource(s): %s", errGet.Error())
		return result, nil // Return error state, but not the error itself to controller
	}

	if len(h.metric.Spec.Projections) == 0 {
		return h.simpleMonitor(ctx, list)
	}
	return h.projectionsMonitor(ctx, list)
}

func (h *MetricHandler) simpleMonitor(ctx context.Context, list *unstructured.UnstructuredList) (MonitorResult, error) {
	primaryCount := len(list.Items)
	dataPoint := clientoptl.NewDataPoint().SetValue(int64(primaryCount))
	h.setDataPointBaseDimensions(dataPoint)

	metricObservation := &v1alpha1.MetricObservation{
		Timestamp:   metav1.Now(),
		LatestValue: strconv.Itoa(len(list.Items)),
	}

	if err := h.gaugeMetric.RecordMetrics(ctx, dataPoint); err != nil {
		// TODO: we should really return the error to the controller and handle it there.
		return MonitorResult{
			Observation: metricObservation,
			Error:       err,
			Phase:       v1alpha1.PhaseFailed,
			Reason:      "RecordMetricFailed",
			Message:     fmt.Sprintf("failed to record metric value: %s", err.Error()),
		}, nil // Return the result, error indicates failure in Monitor execution, not necessarily metric export failure (handled by controller)
	}
	return MonitorResult{
		Observation: metricObservation,
		Phase:       v1alpha1.PhaseActive,
		Reason:      "MonitoringActive",
		Message:     fmt.Sprintf("metric value recorded for resource '%s'", h.metric.GvkToString()),
	}, nil
}

func (h *MetricHandler) projectionsMonitor(ctx context.Context, list *unstructured.UnstructuredList) (MonitorResult, error) {
	groups := h.extractProjectionGroupsFrom(list)
	result := MonitorResult{Observation: &v1alpha1.MetricObservation{Timestamp: metav1.Now()}}

	dataPoints := make([]*clientoptl.DataPoint, 0, len(groups))
	var recordErrors []error

	for _, group := range groups {
		groupCount := len(group)
		dataPoint := clientoptl.NewDataPoint().SetValue(int64(groupCount))

		// Add base dimensions only if they have a non-empty value
		h.setDataPointBaseDimensions(dataPoint)

		// Add projected dimensions for this specific group
		for _, pField := range group {
			// Add projected dimension only if the value is non-empty and no error occurred
			if pField.error == nil && pField.value != "" {
				dataPoint.AddDimension(pField.name, pField.value)
			} else {
				// Optionally log or handle projection errors
				recordErrors = append(recordErrors, fmt.Errorf("projection error for %s: %w", pField.name, pField.error))
			}
		}
		dataPoints = append(dataPoints, dataPoint)
	}

	// Record all collected data points
	errRecord := h.gaugeMetric.RecordMetrics(ctx, dataPoints...)
	if errRecord != nil {
		recordErrors = append(recordErrors, errRecord)
	}

	// Update result based on errors during projection or recording
	if len(recordErrors) > 0 {
		// Combine errors for reporting
		combinedError := fmt.Errorf("errors during metric recording: %v", recordErrors)
		result.Error = combinedError
		result.Phase = v1alpha1.PhaseFailed
		result.Reason = "RecordMetricFailed"
		result.Message = fmt.Sprintf("failed to record metric value(s): %s", combinedError.Error())
	} else {
		result.Phase = v1alpha1.PhaseActive
		result.Reason = v1alpha1.ReasonMonitoringActive
		result.Message = fmt.Sprintf("metric values recorded for resource '%s'", h.metric.GvkToString())
		// Observation might need adjustment depending on how results should be represented in status
		result.Observation = &v1alpha1.MetricObservation{Timestamp: metav1.Now(), LatestValue: strconv.Itoa(len(list.Items))} // Report total count for now
	}
	// Return the result, error indicates failure in Monitor execution, not necessarily metric export failure (handled by controller)
	return result, nil
}

func (h *MetricHandler) setDataPointBaseDimensions(dataPoint *clientoptl.DataPoint) {
	if h.metric.Spec.Target.Kind != "" {
		dataPoint.AddDimension(RESOURCE, h.metric.Spec.Target.Kind)
	}
	if h.metric.Spec.Target.Group != "" {
		dataPoint.AddDimension(GROUP, h.metric.Spec.Target.Group)
	}
	if h.metric.Spec.Target.Version != "" {
		dataPoint.AddDimension(VERSION, h.metric.Spec.Target.Version)
	}
	if h.clusterName != nil && *h.clusterName != "" {
		dataPoint.AddDimension(CLUSTER, *h.clusterName)
	}
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

func (h *MetricHandler) extractProjectionGroupsFrom(list *unstructured.UnstructuredList) map[string][]projectedField {
	// note: for now we only allow one projection, so we can use the first one
	// the reason for this is that if we have multiple projections, we need to create a cartesian product of all projections
	// this is to be done at a later time
	var collection []projectedField

	for _, obj := range list.Items {

		projection := lo.FirstOr(h.metric.Spec.Projections, v1alpha1.Projection{})

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

func (h *MetricHandler) getResources(ctx context.Context) (*unstructured.UnstructuredList, error) {
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

	gvr, err := GetGVRfromGVK(h.metric.Spec.Target.GVK(), h.discoClient)
	if err != nil {
		return nil, err
	}
	list, err := h.dCli.Resource(gvr).List(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("could not find any matching resources for metric set with filter '%s'. %w", gvr.String(), err)
	}

	return list, nil
}

// NewMetricHandler creates a new MetricHandler
func NewMetricHandler(metric v1alpha1.Metric, qc QueryConfig, gaugeMetric *clientoptl.Metric) (*MetricHandler, error) { // Changed dtClient to gaugeMetric
	dynamicClient, errCli := dynamic.NewForConfig(&qc.RestConfig)
	if errCli != nil {
		return nil, errCli
	}

	disco, errDisco := discovery.NewDiscoveryClientForConfig(&qc.RestConfig)
	if errDisco != nil {
		return nil, errDisco
	}

	var handler = &MetricHandler{
		metric:      metric,
		dCli:        dynamicClient,
		discoClient: disco,
		gaugeMetric: gaugeMetric,
		clusterName: qc.ClusterName,
	}

	return handler, nil
}

// GetGVRfromGVK converts GVK to GVR
func GetGVRfromGVK(gvk schema.GroupVersionKind, disco discovery.DiscoveryInterface) (schema.GroupVersionResource, error) {
	// TODO: this could be optimized later (e.g. by caching the discovery client)
	groupResources, err := disco.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		return schema.GroupVersionResource{}, err
	}

	for _, resource := range groupResources.APIResources {
		if strings.EqualFold(resource.Kind, gvk.Kind) {
			return schema.GroupVersionResource{
				Group:    gvk.Group,
				Version:  gvk.Version,
				Resource: resource.Name,
			}, nil
		}
	}

	return schema.GroupVersionResource{}, nil
}

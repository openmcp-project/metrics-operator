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
	"github.com/SAP/metrics-operator/api/v1beta1"
	"github.com/SAP/metrics-operator/internal/clientoptl" // Added
)

// CompoundHandler is used to monitor a compound metric
type CompoundHandler struct {
	dCli        dynamic.Interface
	discoClient discovery.DiscoveryInterface

	metric v1beta1.CompoundMetric

	gaugeMetric *clientoptl.Metric // Changed from dtClient
	clusterName *string
}

// Monitor is used to monitor the metric
func (h *CompoundHandler) Monitor(ctx context.Context) (MonitorResult, error) {

	// Metric creation and export are handled by the controller.
	// This handler focuses on fetching resources, grouping, and recording data points.
	result := MonitorResult{Observation: &v1beta1.MetricObservation{Timestamp: metav1.Now()}}

	list, errGet := h.getResources(ctx)
	if errGet != nil {
		result.Error = errGet
		result.Phase = v1alpha1.PhaseFailed
		result.Reason = "GetResourcesFailed"
		result.Message = fmt.Sprintf("failed to retrieve target resource(s): %s", errGet.Error())
		return result, nil // Return error state, but not the error itself to controller
	}

	groups := h.extractProjectionGroupsFrom(list)

	dataPoints := make([]*clientoptl.DataPoint, 0, len(groups))
	var recordErrors []error

	for _, group := range groups {
		groupCount := len(group)
		dataPoint := clientoptl.NewDataPoint().
			AddDimension(RESOURCE, h.metric.Spec.Target.Resource).
			AddDimension(GROUP, h.metric.Spec.Target.Group).
			AddDimension(VERSION, h.metric.Spec.Target.Version).
			SetValue(int64(groupCount))

		if h.clusterName != nil {
			dataPoint.AddDimension(CLUSTER, *h.clusterName)
		}

		// Add projected dimensions for this specific group
		for _, pField := range group {
			if pField.error == nil {
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
		result.Message = fmt.Sprintf("metric values recorded for resource '%s'", h.metric.Spec.Target.String())
		// Observation might need adjustment depending on how compound results should be represented in status
		result.Observation = &v1beta1.MetricObservation{Timestamp: metav1.Now(), LatestValue: strconv.Itoa(len(list.Items))} // Report total count for now
	}

	// Return the result, error indicates failure in Monitor execution, not necessarily metric export failure (handled by controller)
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

// Removed createGvrBaseMetric as it's clientlite specific

func (h *CompoundHandler) getResources(ctx context.Context) (*unstructured.UnstructuredList, error) {
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
	list, err := h.dCli.Resource(gvr).List(ctx, options)

	if err != nil {
		return nil, fmt.Errorf("could not find any matching resources for metric set with filter '%s'. %w", gvr.String(), err)
	}

	return list, nil
}

// NewCompoundHandler creates a new CompoundHandler
func NewCompoundHandler(metric v1beta1.CompoundMetric, qc QueryConfig, gaugeMetric *clientoptl.Metric) (*CompoundHandler, error) { // Changed dtClient to gaugeMetric
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
		gaugeMetric: gaugeMetric, // Changed dtClient to gaugeMetric
		clusterName: qc.ClusterName,
	}

	return handler, nil
}

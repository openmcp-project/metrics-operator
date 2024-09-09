package metric_orchestratorV2

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1alpha1"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1beta1"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/internal/clientoptl"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

func NewFederatedHandler(metric v1beta1.FederatedMetric, qc QueryConfig, gaugeMetric *clientoptl.Metric) (*FederatedHandler, error) {
	dynamicClient, errCli := dynamic.NewForConfig(&qc.RestConfig)
	if errCli != nil {
		return nil, errCli
	}

	disco, errDisco := discovery.NewDiscoveryClientForConfig(&qc.RestConfig)
	if errDisco != nil {
		return nil, errDisco
	}

	var handler = &FederatedHandler{
		metric:      metric,
		dCli:        dynamicClient,
		discoClient: disco,
		gauge:       gaugeMetric,
		clusterName: qc.ClusterName,
	}

	return handler, nil
}

type FederatedHandler struct {
	dCli        dynamic.Interface
	discoClient discovery.DiscoveryInterface

	metric v1beta1.FederatedMetric

	gauge       *clientoptl.Metric
	clusterName *string
}

func (h *FederatedHandler) Monitor() (MonitorResult, error) {

	result := MonitorResult{}

	list, notFound, err := h.getResources()

	if notFound {
		result.Error = err
		result.Phase = v1alpha1.PhaseFailed
		result.Reason = "ResourceNotFound"
		result.Message = fmt.Sprintf("could not find any matching resources for metric set with filter '%s'", h.metric.Spec.Target.String())
		return result, nil
	}

	if err != nil {
		return MonitorResult{}, fmt.Errorf("could not retrieve target resource(s) %w", err)
	}

	groups := h.extractProjectionGroupsFrom(list)

	var dimensions []v1beta1.Dimension

	for _, group := range groups {
		dp := clientoptl.NewDataPoint().
			AddDimension(CLUSTER, *h.clusterName).
			AddDimension(RESOURCE, h.metric.Spec.Target.Resource).
			AddDimension(GROUP, h.metric.Spec.Target.Group).
			AddDimension(VERSION, h.metric.Spec.Target.Version).
			SetValue(int64(len(group)))

		for _, pField := range group {
			if pField.error == nil {

				// empty values will be ignored and rejected by the opentelemetry collector, need to give it some value to avoid this
				if pField.value == "" {
					pField.value = "n/a"
				}
				dp.AddDimension(pField.name, pField.value)
				dimensions = append(dimensions, v1beta1.Dimension{Name: pField.name, Value: pField.value})
			}
		}
		err = h.gauge.RecordMetrics(dp)
		if err != nil {
			return MonitorResult{}, fmt.Errorf("could not record metric: %w", err)
		}
	}

	//err = h.mCli.ExportMetrics(context.Background())

	result.Phase = v1alpha1.PhaseActive
	result.Reason = "MonitoringActive"
	result.Message = fmt.Sprintf("metric is monitoring resource '%s'", h.metric.Spec.Target.String())

	if dimensions != nil {
		result.Observation = &v1beta1.MetricObservation{Timestamp: metav1.Now(), Dimensions: []v1beta1.Dimension{{Name: dimensions[0].Name, Value: strconv.Itoa(len(list.Items))}}}
	} else {
		result.Observation = &v1beta1.MetricObservation{Timestamp: metav1.Now()}
	}

	return result, nil
}

func (h *FederatedHandler) extractProjectionGroupsFrom(list *unstructured.UnstructuredList) map[string][]projectedField {

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

func (h *FederatedHandler) getResources() (*unstructured.UnstructuredList, bool, error) {
	var options = metav1.ListOptions{}
	// if not defined in the metric, the list options need to be empty to get resources based on GVR only
	if h.metric.Spec.Selectors.LabelSelector != "" && h.metric.Spec.Selectors.FieldSelector != "" {
		options = metav1.ListOptions{LabelSelector: h.metric.Spec.Selectors.LabelSelector, FieldSelector: h.metric.Spec.Selectors.FieldSelector}
	}
	gvr := schema.GroupVersionResource{
		Group:    h.metric.Spec.Target.Group,
		Version:  h.metric.Spec.Target.Version,
		Resource: h.metric.Spec.Target.Resource,
	}
	list, err := h.dCli.Resource(gvr).List(context.Background(), options)

	if err != nil {
		if isDNSLookupError(err) || apierrors.IsNotFound(err) {
			return nil, true, fmt.Errorf("could not find any matching resources for metric set with filter '%s'. %w", gvr.String(), err)
		}
		return nil, false, fmt.Errorf("could not find any matching resources for metric set with filter '%s'. %w", gvr.String(), err)
	}

	// Group resources by name
	groupedResources := lo.GroupBy(list.Items, func(item unstructured.Unstructured) string {
		return item.GetName()
	})

	// Get the latest generation for each group
	latestResources := lo.MapValues(groupedResources, func(items []unstructured.Unstructured, _ string) unstructured.Unstructured {
		return lo.MaxBy(items, func(a, b unstructured.Unstructured) bool {
			genA, existsA, _ := unstructured.NestedInt64(a.Object, "metadata", "generation")
			genB, existsB, _ := unstructured.NestedInt64(b.Object, "metadata", "generation")

			// If generation doesn't exist for either, compare by resource version
			if !existsA || !existsB {
				return a.GetResourceVersion() > b.GetResourceVersion()
			}

			return genA > genB
		})
	})

	// Convert map to slice
	latestResourcesList := lo.Values(latestResources)

	// Create a new UnstructuredList with only the latest generation of each resource
	filteredList := &unstructured.UnstructuredList{
		Items: latestResourcesList,
	}
	// Copy the rest of the fields from the original list
	filteredList.SetAPIVersion(list.GetAPIVersion())
	filteredList.SetKind(list.GetKind())
	filteredList.SetResourceVersion(list.GetResourceVersion())
	filteredList.SetContinue(list.GetContinue())
	filteredList.SetRemainingItemCount(list.GetRemainingItemCount())

	return filteredList, false, nil
}
func isDNSLookupError(err error) bool {
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return dnsError.IsNotFound
	}

	// Fallback to string matching if error type assertion fails
	return strings.Contains(err.Error(), "no such host")
}

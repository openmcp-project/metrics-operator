package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/samber/lo"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"github.com/openmcp-project/metrics-operator/internal/clientoptl"
)

// NewFederatedHandler creates a new FederatedHandler
func NewFederatedHandler(metric v1alpha1.FederatedMetric, qc QueryConfig, gaugeMetric *clientoptl.Metric) (*FederatedHandler, error) {
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

// FederatedHandler is used to monitor the metric
type FederatedHandler struct {
	dCli        dynamic.Interface
	discoClient discovery.DiscoveryInterface

	metric v1alpha1.FederatedMetric

	gauge       *clientoptl.Metric
	clusterName *string
}

// Monitor is used to monitor the metric
func (h *FederatedHandler) Monitor(ctx context.Context) (MonitorResult, error) {

	result := MonitorResult{}

	list, notFound, err := h.getResources(ctx)

	if notFound {
		result.Error = err
		result.Phase = v1alpha1.PhaseFailed
		result.Reason = "ResourceNotFound"
		result.Message = fmt.Sprintf("could not find any matching resources for metric set with filter '%s'", h.metric.Spec.Target.GVK().String())
		return result, nil
	}

	if err != nil {
		return MonitorResult{}, fmt.Errorf("could not retrieve target resource(s) %w", err)
	}

	groups := ExtractProjectionGroupsFrom(list, h.metric.Spec.Projections)
	dimensions := make(map[string]int)

	for _, fieldGroups := range groups {
		// Calculate count as the number of resource instances with this combination
		count := len(fieldGroups)

		dp := clientoptl.NewDataPoint().
			AddDimension(CLUSTER, *h.clusterName).
			AddDimension(RESOURCE, h.metric.Spec.Target.Kind).
			AddDimension(GROUP, h.metric.Spec.Target.Group).
			AddDimension(VERSION, h.metric.Spec.Target.Version).
			SetValue(int64(count))

		if len(fieldGroups) > 0 {
			for _, pField := range fieldGroups[0] {
				if pField.Error == nil {
					// empty values will be ignored and rejected by the opentelemetry collector, need to give it some Value to avoid this
					value := pField.Value
					if value == "" {
						value = "n/a"
					}
					dp.AddDimension(pField.Name, value)
					dimensions[pField.Name] = dimensions[pField.Name] + count
				}
			}
		}

		err = h.gauge.RecordMetrics(ctx, dp)
		if err != nil {
			return MonitorResult{}, fmt.Errorf("could not record metric: %w", err)
		}
	}

	// err = h.mCli.ExportMetrics(context.Background())

	result.Phase = v1alpha1.PhaseActive
	result.Reason = v1alpha1.ReasonMonitoringActive
	result.Message = fmt.Sprintf("metric is monitoring resource '%s'", h.metric.Spec.Target.GVK().String())

	if len(dimensions) > 0 {
		observation := &v1alpha1.MetricObservation{
			Timestamp:  metav1.Now(),
			Dimensions: make([]v1alpha1.Dimension, 0, len(dimensions)),
		}
		for name, count := range dimensions {
			observation.Dimensions = append(observation.Dimensions, v1alpha1.Dimension{
				Name:  name,
				Value: strconv.Itoa(count),
			})
		}
		result.Observation = observation
	} else {
		result.Observation = &v1alpha1.MetricObservation{Timestamp: metav1.Now()}
	}

	return result, nil
}

type projectionGroups map[string][][]projectedField

// extractProjectionGroupsFrom takes a list of unstructured objects and a list of projections,
// It returns a map where the key is a unique combination of projected values and the value is a list of groups of projected fields that share that combination.
func extractProjectionGroupsFrom(list *unstructured.UnstructuredList, projections []v1alpha1.Projection) projectionGroups {
	collection := make([][]projectedField, 0, len(list.Items))

	for _, obj := range list.Items {
		var fields []projectedField
		for _, projection := range projections {
			if projection.Name != "" && projection.FieldPath != "" {
				name := projection.Name
				value, found, err := nestedFieldValue(obj, projection.FieldPath, v1alpha1.DimensionType(projection.Type))
				fields = append(fields, projectedField{name: name, value: value, found: found, error: err})
			}
		}
		if fields != nil {
			collection = append(collection, fields)
		}
	}

	// Group by the combination of all projected values (cartesian product)
	groups := lo.GroupBy(collection, func(fields []projectedField) string {
		keyParts := make([]string, 0, len(fields))
		for _, f := range fields {
			keyParts = append(keyParts, f.GetID())
		}
		return strings.Join(keyParts, ",")
	})

	return groups
}

	// Group by the combination of all projected values (cartesian product)
	groups := lo.GroupBy(collection, func(fields []ProjectedField) string {
		keyParts := make([]string, 0, len(fields))
		for _, f := range fields {
			keyParts = append(keyParts, f.GetID())
		}
		return strings.Join(keyParts, ",")
	})

	return groups
}

func (h *FederatedHandler) getResources(ctx context.Context) (*unstructured.UnstructuredList, bool, error) {
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
		return nil, false, fmt.Errorf("failed to get target GVK: %w", err)
	}

	list, err := h.dCli.Resource(gvr).List(ctx, options)

	if err != nil {
		if isDNSLookupError(err) || apierrors.IsNotFound(err) {
			return nil, true, fmt.Errorf("could not find any matching resources for metric set with filter '%s'. %w", gvr.String(), err)
		}
		return nil, false, fmt.Errorf("could not find any matching resources for metric set with filter '%s'. %w", gvr.String(), err)
	}

	// Group resources by namespace/Name
	groupedResources := lo.GroupBy(list.Items, func(item unstructured.Unstructured) string {
		return fmt.Sprintf("%s/%s", item.GetNamespace(), item.GetName())
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

	// Fallback to string matching if Error type assertion fails
	return strings.Contains(err.Error(), "no such host")
}

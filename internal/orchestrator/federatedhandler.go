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
		clusterKey:  discoveryCacheKeyFromConfig(qc),
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
	clusterKey  string
}

// Monitor is used to monitor the metric
func (h *FederatedHandler) Monitor(ctx context.Context) (MonitorResult, error) {

	result := MonitorResult{}

	list, lookup, notFound, err := h.getResources(ctx)

	if notFound {
		result.Error = err
		result.Phase = v1alpha1.PhaseFailed
		result.Reason = "ResourceNotFound"
		result.Message = fmt.Sprintf("could not find any matching resources for metric set with filter '%s'", h.metric.Spec.Target.GVK().String())
		result.Observation = &v1alpha1.MetricObservation{
			Timestamp: metav1.Now(),
			CommonObservation: v1alpha1.CommonObservation{
				TargetLookupDurationMillis: lookup.Duration.Milliseconds(),
				TargetLookupCacheHits:      lookup.CacheHits,
			},
		}
		return result, nil
	}

	if err != nil {
		return MonitorResult{}, fmt.Errorf("could not retrieve target resource(s) %w", err)
	}

	groups := extractProjectionGroupsFromWithResourceGVK(list, h.metric.Spec.Projections, true)
	valueByUID := resolveValueFrom(list, h.metric.Spec.ValueFrom)
	dimensions := make(map[string]int)

	for _, fieldGroups := range groups {
		// Calculate count as the number of resource instances with this combination
		count := len(fieldGroups)

		dp := clientoptl.NewDataPoint().SetValue(int64(count))
		addClusterDimension(dp, h.clusterName)
		addTargetGVKDimensions(dp, &h.metric.Spec.Target)

		if len(fieldGroups) > 0 {
			// Use aggregated valueFrom across all objects in the group if available
			uids := make([]string, 0, len(fieldGroups))
			for _, inGroup := range fieldGroups {
				if len(inGroup) > 0 {
					uids = append(uids, inGroup[0].uid)
				}
			}
			if v, ok := aggregateGroupValue(uids, valueByUID, h.metric.Spec.ValueFrom); ok {
				dp.SetValue(v)
			}
			for _, pField := range fieldGroups[0] {
				if pField.error == nil {
					// empty values will be ignored and rejected by the opentelemetry collector, need to give it some Value to avoid this
					value := pField.value
					if value == "" {
						value = "n/a"
					}
					dp.AddDimension(pField.name, value)
					dimensions[pField.name] = dimensions[pField.name] + count
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
			CommonObservation: v1alpha1.CommonObservation{
				TargetLookupDurationMillis: lookup.Duration.Milliseconds(),
				TargetLookupCacheHits:      lookup.CacheHits,
			},
		}
		for name, count := range dimensions {
			observation.Dimensions = append(observation.Dimensions, v1alpha1.Dimension{
				Name:  name,
				Value: strconv.Itoa(count),
			})
		}
		result.Observation = observation
	} else {
		result.Observation = &v1alpha1.MetricObservation{
			Timestamp: metav1.Now(),
			CommonObservation: v1alpha1.CommonObservation{
				TargetLookupDurationMillis: lookup.Duration.Milliseconds(),
				TargetLookupCacheHits:      lookup.CacheHits,
			},
		}
	}

	return result, nil
}

func (h *FederatedHandler) getResources(ctx context.Context) (*unstructured.UnstructuredList, targetLookupResult, bool, error) {
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

	lookup, err := lookupTargetResources(h.metric.Spec.Target, h.metric.Status.Observation.CommonObservation, h.discoClient, h.clusterKey)
	if err != nil {
		return nil, lookup, false, fmt.Errorf("failed to get target GVK: %w", err)
	}

	items := make([]unstructured.Unstructured, 0)
	for _, resource := range lookup.Resources {
		list, err := h.dCli.Resource(resource.GVR).List(ctx, options)
		if err != nil {
			wrapped := fmt.Errorf("could not find any matching resources for metric set with filter '%s'. %w", resource.GVR.String(), err)
			if isDNSLookupError(err) || apierrors.IsNotFound(err) {
				return nil, lookup, true, wrapped
			}
			return nil, lookup, false, wrapped
		}
		for _, item := range list.Items {
			item.SetGroupVersionKind(resource.GVK)
			items = append(items, item)
		}
	}

	// Group resources by GVK/namespace/name
	groupedResources := lo.GroupBy(items, func(item unstructured.Unstructured) string {
		return fmt.Sprintf("%s/%s/%s/%s", item.GetAPIVersion(), item.GetKind(), item.GetNamespace(), item.GetName())
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

	return filteredList, lookup, false, nil
}
func isDNSLookupError(err error) bool {
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return dnsError.IsNotFound
	}

	// Fallback to string matching if Error type assertion fails
	return strings.Contains(err.Error(), "no such host")
}

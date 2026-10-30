package orchestrator

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	rcli "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"github.com/openmcp-project/metrics-operator/internal/clientoptl"
)

const federatedManagedListPageSize int64 = 500

// NewFederatedManagedHandler creates a new FederatedManagedHandler
func NewFederatedManagedHandler(metric v1alpha1.FederatedManagedMetric, qc QueryConfig, gaugeMetric *clientoptl.Metric) (*FederatedManagedHandler, error) {
	dynamicClient, errCli := dynamic.NewForConfig(&qc.RestConfig)
	if errCli != nil {
		return nil, errCli
	}

	var handler = &FederatedManagedHandler{
		client:      qc.Client,
		metric:      metric,
		dCli:        dynamicClient,
		gauge:       gaugeMetric,
		clusterName: qc.ClusterName,
	}

	return handler, nil
}

// FederatedManagedHandler is used to monitor the metric
type FederatedManagedHandler struct {
	client rcli.Client
	dCli   dynamic.Interface

	metric v1alpha1.FederatedManagedMetric

	gauge       *clientoptl.Metric
	clusterName *string
}

// Monitor is used to monitor the metric
func (h *FederatedManagedHandler) Monitor(ctx context.Context) (MonitorResult, error) {
	result := MonitorResult{}

	count, err := h.recordManagedResourceCounts(ctx)
	if err != nil {
		result.Error = err
		result.Phase = v1alpha1.PhaseFailed
		result.Reason = "ResourceNotFound"
		result.Message = fmt.Sprintf("could not find any matching federated managed resources for metric '%s'", h.metric.Spec.Name)
		return result, nil //nolint:nilerr
	}

	result.Phase = v1alpha1.PhaseActive
	result.Reason = v1alpha1.ReasonMonitoringActive
	result.Message = fmt.Sprintf("metric is monitoring federated managed resources '%s'", h.metric.Name)
	result.Observation = &v1alpha1.MetricObservation{
		Timestamp:   metav1.Now(),
		LatestValue: strconv.Itoa(count),
		Dimensions:  []v1alpha1.Dimension{{Name: "resources", Value: strconv.Itoa(count)}},
	}

	return result, nil
}

func (h *FederatedManagedHandler) recordManagedResourceCounts(ctx context.Context) (int, error) {
	crds := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := h.client.List(ctx, crds); err != nil {
		return 0, err
	}

	type bucket struct {
		count  int64
		fields []projectedField
	}
	type resourceBuckets struct {
		gvk     schema.GroupVersionKind
		buckets map[string]*bucket
	}
	byGVK := make(map[schema.GroupVersionKind]*resourceBuckets)
	resourceCount := 0
	projections := federatedManagedConditionProjections()

	for _, crd := range crds.Items {
		if !slices.Contains(crd.Spec.Names.Categories, "crossplane") || !slices.Contains(crd.Spec.Names.Categories, "managed") {
			continue
		}
		for _, crdv := range crd.Spec.Versions {
			if !crdv.Served || !crdv.Storage {
				continue
			}
			gvk := schema.GroupVersionKind{Group: crd.Spec.Group, Version: crdv.Name, Kind: crd.Spec.Names.Kind}
			resourceSet := byGVK[gvk]
			if resourceSet == nil {
				resourceSet = &resourceBuckets{gvk: gvk, buckets: make(map[string]*bucket)}
				byGVK[gvk] = resourceSet
			}
			gvr := schema.GroupVersionResource{Resource: crd.Spec.Names.Plural, Group: crd.Spec.Group, Version: crdv.Name}
			opts := metav1.ListOptions{Limit: federatedManagedListPageSize}
			for {
				list, err := h.dCli.Resource(gvr).List(ctx, opts)
				if err != nil {
					return 0, fmt.Errorf("could not find any matching resources for metric '%s'. %w", h.metric.Name, err)
				}
				groups := extractProjectionGroupsFrom(list, projections)
				for key, group := range groups {
					merged := resourceSet.buckets[key]
					if merged == nil {
						merged = &bucket{fields: group[0]}
						resourceSet.buckets[key] = merged
					}
					merged.count += int64(len(group))
					resourceCount += len(group)
				}
				if list.GetContinue() == "" {
					break
				}
				opts.Continue = list.GetContinue()
			}
		}
	}

	for _, resourceBuckets := range byGVK {
		for _, group := range resourceBuckets.buckets {
			dp := clientoptl.NewDataPoint().SetValue(group.count)
			dp.AddDimension(GROUP, resourceBuckets.gvk.Group)
			dp.AddDimension(VERSION, resourceBuckets.gvk.Version)
			dp.AddDimension(KIND, resourceBuckets.gvk.Kind)
			dp.AddDimension(APIVERSION, resourceBuckets.gvk.GroupVersion().String())
			if h.clusterName != nil && *h.clusterName != "" {
				dp.AddDimension(CLUSTER, *h.clusterName)
			}
			for _, field := range group.fields {
				if field.error == nil && field.value != "" {
					dp.AddDimension(field.name, field.value)
				}
			}
			if err := h.gauge.RecordMetrics(ctx, dp); err != nil {
				return resourceCount, fmt.Errorf("could not record metric: %w", err)
			}
		}
	}

	return resourceCount, nil
}

func federatedManagedConditionProjections() []v1alpha1.Projection {
	unknown := v1alpha1.NewProjectionDefaultValue("unknown")
	return []v1alpha1.Projection{
		{Name: "Ready", FieldPath: "status.conditions[?(@.type=='Ready')].status", Type: v1alpha1.TypePrimitive, Default: unknown},
		{Name: "Synced", FieldPath: "status.conditions[?(@.type=='Synced')].status", Type: v1alpha1.TypePrimitive, Default: unknown},
	}
}

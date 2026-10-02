package orchestrator

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	rcli "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"github.com/openmcp-project/metrics-operator/internal/clientoptl"
)

// ManagedHandler is used to monitor the metric
type ManagedHandler struct {
	client rcli.Client
	dCli   dynamic.Interface

	metric      v1alpha1.ManagedMetric
	gaugeMetric *clientoptl.Metric

	clusterName *string
}

// NewManagedHandler creates a new ManagedHandler
func NewManagedHandler(metric v1alpha1.ManagedMetric, qc QueryConfig, gaugeMetric *clientoptl.Metric) (*ManagedHandler, error) {
	dynamicClient, errCli := dynamic.NewForConfig(&qc.RestConfig)
	if errCli != nil {
		return nil, fmt.Errorf("could not create dynamic client: %w", errCli)
	}

	var handler = &ManagedHandler{
		client:      qc.Client,
		dCli:        dynamicClient,
		metric:      metric,
		gaugeMetric: gaugeMetric,
		clusterName: qc.ClusterName,
	}

	return handler, nil
}

func (h *ManagedHandler) sendStatusBasedMetricValue(ctx context.Context) (string, error) {
	list, err := h.getManagedUnstructured(ctx)
	if err != nil {
		return "", err
	}

	projections := h.metric.Spec.Dimensions
	if projections == nil {
		projections = defaultManagedProjections()
	}
	projections = managedCustomProjections(projections)

	byGVK := make(map[schema.GroupVersionKind]*unstructured.UnstructuredList)
	for i := range list.Items {
		obj := &list.Items[i]
		gvk := obj.GroupVersionKind()
		group := byGVK[gvk]
		if group == nil {
			group = &unstructured.UnstructuredList{}
			byGVK[gvk] = group
		}
		group.Items = append(group.Items, *obj)
	}

	for gvk, resources := range byGVK {
		groups := extractProjectionGroupsFrom(resources, projections)
		if len(projections) == 0 {
			fields := make([][]projectedField, len(resources.Items))
			groups = projectionGroups{"": fields}
		}
		for _, group := range groups {
			dp := clientoptl.NewDataPoint().SetValue(int64(len(group)))
			dp.AddDimension(KIND, gvk.Kind)
			dp.AddDimension(GROUP, gvk.Group)
			dp.AddDimension(VERSION, gvk.Version)
			if h.clusterName != nil && *h.clusterName != "" {
				dp.AddDimension(CLUSTER, *h.clusterName)
			}
			if len(group) > 0 {
				for _, pf := range group[0] {
					if pf.error == nil && pf.value != "" {
						dp.AddDimension(pf.name, pf.value)
					}
				}
			}
			if err := h.gaugeMetric.RecordMetrics(ctx, dp); err != nil {
				return "", err
			}
		}
	}
	return strconv.Itoa(len(list.Items)), nil
}

func managedCustomProjections(projections []v1alpha1.Projection) []v1alpha1.Projection {
	filtered := make([]v1alpha1.Projection, 0, len(projections))
	for _, projection := range projections {
		switch projection.Name {
		case KIND, GROUP, VERSION, CLUSTER:
			continue
		default:
			filtered = append(filtered, projection)
		}
	}
	return filtered
}

// defaultManagedProjections returns the default ready/synced projections for Managed* resources.
// These preserve backwards-compatible label dimensions when no custom Dimensions are configured.
func defaultManagedProjections() []v1alpha1.Projection {
	return []v1alpha1.Projection{
		{Name: "ready", FieldPath: "status.conditions[?(@.type=='Ready')].status", Type: v1alpha1.TypePrimitive},
		{Name: "synced", FieldPath: "status.conditions[?(@.type=='Synced')].status", Type: v1alpha1.TypePrimitive},
	}
}

// getManagedUnstructured returns the raw unstructured list of all matching managed resources.
func (h *ManagedHandler) getManagedUnstructured(ctx context.Context) (*unstructured.UnstructuredList, error) {
	crds := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := h.client.List(ctx, crds); err != nil {
		return nil, err
	}

	result := &unstructured.UnstructuredList{Items: make([]unstructured.Unstructured, 0)}
	for _, crd := range crds.Items {
		if !h.hasCategory("crossplane", crd) || !h.hasCategory("managed", crd) {
			continue
		}
		if !h.matchesGroupVersionKind(crd) {
			continue
		}
		for _, crdv := range crd.Spec.Versions {
			if !crdv.Served {
				continue
			}
			target := h.metric.Spec.Target
			if target != nil && target.Version != "" && target.Version != crdv.Name {
				continue
			}
			gvr := schema.GroupVersionResource{
				Resource: crd.Spec.Names.Plural,
				Group:    crd.Spec.Group,
				Version:  crdv.Name,
			}
			list, err := h.dCli.Resource(gvr).List(ctx, metav1.ListOptions{})
			if err != nil {
				return nil, fmt.Errorf("could not find any matching resources for metric with filter '%s'. %w", h.metric.GvkToString(), err)
			}
			result.Items = append(result.Items, list.Items...)
		}
	}
	return result, nil
}

// Monitor executes the monitoring of the metric
func (h *ManagedHandler) Monitor(ctx context.Context) (MonitorResult, error) {
	result := MonitorResult{}
	resources, err := h.sendStatusBasedMetricValue(ctx)

	if err != nil {
		result.Error = err
		result.Phase = v1alpha1.PhaseFailed
		result.Reason = "SendMetricFailed"
		result.Message = fmt.Sprintf("failed to send metric value to data sink. %s", err.Error())
	} else {
		result.Phase = v1alpha1.PhaseActive
		result.Observation = &v1alpha1.ManagedObservation{Timestamp: metav1.Now(), Resources: resources}
		result.Reason = v1alpha1.ReasonMonitoringActive
		result.Message = fmt.Sprintf("metric is monitoring resource '%s'", h.metric.GvkToString())
	}

	return result, nil
}

// is used to check if a resource from the cluster has a specific field
func (h *ManagedHandler) hasCategory(category string, crd apiextensionsv1.CustomResourceDefinition) bool {
	for _, v := range crd.Spec.Names.Categories {
		if v == category {
			return true
		}
	}

	return false
}

func (h *ManagedHandler) matchesGroupVersionKind(crd apiextensionsv1.CustomResourceDefinition) bool {
	target := h.metric.Spec.Target
	// if the user does not specify a GVK target, any managed CRD is considered a match
	if target == nil {
		return true
	}
	// CRDs may define multi-version APIs
	// we consider a version to be a match if it exists in a CRD
	crdVersions := make([]string, 0, len(crd.Spec.Versions))
	for _, version := range crd.Spec.Versions {
		crdVersions = append(crdVersions, version.Name)
	}
	// if the user specifies a target, we consider each GVK attribute and check if it matches the user value
	// if the user does not specify a single GVK part, that part is considered unconditional and always a match
	if target.Version != "" && !slices.Contains(crdVersions, target.Version) {
		return false
	}
	if target.Group != "" && target.Group != crd.Spec.Group {
		return false
	}
	if target.Kind != "" && target.Kind != crd.Spec.Names.Kind {
		return false
	}
	return true
}

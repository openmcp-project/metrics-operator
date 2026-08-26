package orchestrator

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
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

	disco, errDisco := discovery.NewDiscoveryClientForConfig(&qc.RestConfig)
	if errDisco != nil {
		return nil, errDisco
	}

	var handler = &FederatedManagedHandler{
		client:      qc.Client,
		metric:      metric,
		dCli:        dynamicClient,
		discoClient: disco,
		gauge:       gaugeMetric,
		clusterName: qc.ClusterName,
	}

	return handler, nil
}

// FederatedManagedHandler is used to monitor the metric
type FederatedManagedHandler struct {
	client      rcli.Client
	dCli        dynamic.Interface
	discoClient discovery.DiscoveryInterface

	metric v1alpha1.FederatedManagedMetric

	gauge       *clientoptl.Metric
	clusterName *string
}

type federatedManagedBucket struct {
	cluster    string
	kind       string
	apiVersion string
	ready      string
	synced     string
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
	crds := &apiextensionsv1.CustomResourceDefinitionList{} // get ALL custom resource definitions
	if err := h.client.List(ctx, crds); err != nil {
		return 0, err
	}

	counts := map[federatedManagedBucket]int64{}
	total := 0

	for _, crd := range crds.Items {
		if !slices.Contains(crd.Spec.Names.Categories, "crossplane") || !slices.Contains(crd.Spec.Names.Categories, "managed") { // filter previously acquired crds
			continue
		}

		// Use the stored versions of the CRD
		storedVersions := make(map[string]bool)
		for _, v := range crd.Status.StoredVersions {
			storedVersions[v] = true
		}

		for _, crdv := range crd.Spec.Versions {
			if !crdv.Served || !storedVersions[crdv.Name] {
				continue
			}

			gvr := schema.GroupVersionResource{
				Resource: crd.Spec.Names.Plural,
				Group:    crd.Spec.Group,
				Version:  crdv.Name,
			}

			opts := metav1.ListOptions{Limit: federatedManagedListPageSize}
			for {
				list, err := h.dCli.Resource(gvr).List(ctx, opts) // gets resources from all the available crds
				if err != nil {
					return total, fmt.Errorf("could not find any matching resources for metric '%s'. %w", h.metric.Name, err)
				}

				for i := range list.Items {
					item := &list.Items[i]
					counts[federatedManagedBucket{
						cluster:    h.clusterDimension(),
						kind:       item.GetKind(),
						apiVersion: item.GetAPIVersion(),
						ready:      conditionStatus(item, "Ready"),
						synced:     conditionStatus(item, "Synced"),
					}]++
					total++
				}

				if list.GetContinue() == "" {
					break
				}
				opts.Continue = list.GetContinue()
			}
		}
	}

	for bucket, count := range counts {
		dataPoint := clientoptl.NewDataPoint().
			AddDimension(CLUSTER, bucket.cluster).
			AddDimension(KIND, bucket.kind).
			AddDimension(APIVERSION, bucket.apiVersion).
			AddDimension("Ready", bucket.ready).
			AddDimension("Synced", bucket.synced).
			SetValue(count)

		if err := h.gauge.RecordMetrics(ctx, dataPoint); err != nil {
			return total, fmt.Errorf("could not record metric: %w", err)
		}
	}

	return total, nil
}

func (h *FederatedManagedHandler) clusterDimension() string {
	if h.clusterName == nil {
		return ""
	}
	return *h.clusterName
}

func conditionStatus(item *unstructured.Unstructured, conditionType string) string {
	conditions, ok, _ := unstructured.NestedSlice(item.Object, "status", "conditions")
	if !ok {
		return "unknown"
	}

	for _, condition := range conditions {
		conditionMap, ok := condition.(map[string]any)
		if !ok {
			continue
		}
		if conditionMap["type"] != conditionType {
			continue
		}
		status, ok := conditionMap["status"].(string)
		if !ok {
			return "unknown"
		}
		return strings.ToLower(status)
	}

	return "unknown"
}

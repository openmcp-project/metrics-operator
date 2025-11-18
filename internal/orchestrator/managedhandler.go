package orchestrator

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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
	resources, err := h.getResourcesStatus(ctx)
	if err != nil {
		return "", err
	}

	// data point split by dimensions
	for _, cr := range resources {
		// Create a new data point for each resource
		dataPoint := clientoptl.NewDataPoint()

		objMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&cr.MangedResource)
		if err != nil {
			return "", err
		}

		u := &unstructured.Unstructured{Object: objMap}

		for key, expr := range h.metric.Spec.Dimensions {
			s, _, err := nestedPrimitiveValue(*u, expr)
			if err != nil {
				fmt.Printf("WARN: Could not parse expression '%s' for dimension field '%s'. Error: %v\n", key, expr, err)
				continue
			}
			dataPoint.AddDimension(key, s)
		}

		// Add cluster dimension if available
		if h.clusterName != nil {
			dataPoint.AddDimension(CLUSTER, *h.clusterName)
		}

		// Set the value to 1 for each resource
		dataPoint.SetValue(1)

		// Record the metric
		err = h.gaugeMetric.RecordMetrics(ctx, dataPoint)
		if err != nil {
			return "", err
		}
	}

	resourcesCount := len(resources)

	// if no err, returns nil...duh!
	return strconv.Itoa(resourcesCount), err
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
		result.Reason = "MonitoringActive"
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

func (h *ManagedHandler) getResourcesStatus(ctx context.Context) ([]ClusterResourceStatus, error) {
	managedResources, err := h.getManagedResources(ctx)
	if err != nil {
		return []ClusterResourceStatus{}, err
	}

	crStatuses := make([]ClusterResourceStatus, 0)

	for _, item := range managedResources {
		rsStatus := ClusterResourceStatus{MangedResource: item, Status: make(map[string]bool)}
		for _, condition := range item.Status.Conditions {
			status, _ := strconv.ParseBool(condition.Status)
			rsStatus.Status[condition.Type] = status
		}
		crStatuses = append(crStatuses, rsStatus)
	}

	return crStatuses, nil
}

//nolint:gocyclo
func (h *ManagedHandler) getManagedResources(ctx context.Context) ([]Managed, error) {

	crds := &apiextensionsv1.CustomResourceDefinitionList{} // get ALL custom resource definitions
	if err := h.client.List(ctx, crds); err != nil {
		return nil, err
	}

	resourceCRDs := make([]apiextensionsv1.CustomResourceDefinition, 0, len(crds.Items))
	for _, crd := range crds.Items {
		// drop non-crossplane crds
		if !h.hasCategory("crossplane", crd) || !h.hasCategory("managed", crd) {
			continue
		}
		// drop crds that don't match the spec gvk
		if !h.matchesGroupVersionKind(crd) {
			continue
		}
		resourceCRDs = append(resourceCRDs, crd)
	}

	var resources []unstructured.Unstructured
	for _, crd := range resourceCRDs {
		versionsToRetrieve := make([]string, 0, len(crd.Spec.Versions))
		for _, crdv := range crd.Spec.Versions {
			// only use served versions for retrieval
			if !crdv.Served {
				continue
			}
			// drop versions that don't match the user provided target
			target := h.metric.Spec.Target
			if target != nil && target.Version != "" && target.Version != crdv.Name {
				continue
			}
			versionsToRetrieve = append(versionsToRetrieve, crdv.Name)
		}
		// finally retrieve all matching resources
		for _, version := range versionsToRetrieve {
			gvr := schema.GroupVersionResource{
				Resource: crd.Spec.Names.Plural,
				Group:    crd.Spec.Group,
				Version:  version,
			}

			list, err := h.dCli.Resource(gvr).List(ctx, metav1.ListOptions{}) // gets resources from all the available crds
			if err != nil {
				return nil, fmt.Errorf("could not find any matching resources for metric with filter '%s'. %w", h.metric.GvkToString(), err)
			}

			if len(list.Items) > 0 {
				resources = append(resources, list.Items...)
			}
		}
	}

	managedResources := make([]Managed, 0, len(resources))
	for _, u := range resources {
		managed := Managed{}
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), &managed)
		if err != nil {
			return nil, err
		}

		managedResources = append(managedResources, managed)
	}

	return managedResources, nil
}

// Managed is a struct that holds the managed resource
type Managed struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Spec       Spec              `json:"spec"`
	Metadata   metav1.ObjectMeta `json:"metadata"`
	Status     Status            `json:"status"`
}

// Status is a struct that holds the status of a resource
type Status struct {
	AtProvider map[string]any `json:"forProvider"`
	Conditions []Condition    `json:"conditions"`
}

// Condition is a struct that holds the condition of a resource
type Condition struct {
	LastTransitionTime string `json:"lastTransitionTime"`
	Message            string `json:"message"`
	Reason             string `json:"reason"`
	Status             string `json:"status"`
	Type               string `json:"type"`
}

// Spec is a struct that holds the specification of a resource
type Spec struct {
	ForProvider map[string]any `json:"forProvider"`
}

// ClusterResourceStatus is a struct that holds the status of a resource in the cluster
type ClusterResourceStatus struct {
	MangedResource Managed
	Status         map[string]bool
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

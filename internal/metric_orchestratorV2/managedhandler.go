package metric_orchestratorV2

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/internal/client"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	rcli "sigs.k8s.io/controller-runtime/pkg/client"
)

type ManagedHandler struct {
	client rcli.Client
	dCli   dynamic.Interface

	metric     v1.ManagedMetric
	metricMeta client.MetricMetadata

	dtClient client.DynatraceClient
}

func NewManagedHandler(metric v1.ManagedMetric, metricMeta client.MetricMetadata, restConfig *rest.Config, client rcli.Client, dtClient client.DynatraceClient) (*ManagedHandler, error) {
	dynamicClient, errCli := dynamic.NewForConfig(restConfig)
	if errCli != nil {
		return nil, fmt.Errorf("could not create dynamic client: %w", errCli)
	}

	var handler = &ManagedHandler{
		client:     client,
		dCli:       dynamicClient,
		metric:     metric,
		metricMeta: metricMeta,
		dtClient:   dtClient,
	}

	return handler, nil
}

func (h *ManagedHandler) sendStatusBasedMetricValue() error {
	// add the Datapoint for the metric
	h.metricMeta.AddDatapoint(1)
	resources, err := h.getResourcesStatus()
	if err != nil {
		return err
	}

	// data point split by dimensions
	for _, cr := range resources {
		h.metricMeta.ClearDimensions()
		_ = h.metricMeta.AddDimension("kind", cr.MangedResource.Kind)
		_ = h.metricMeta.AddDimension("apiversion", cr.MangedResource.APIVersion)

		// TODO: add mcp name as well later
		// b.dynaMetric.AddDimension("name", ...)

		for typ, state := range cr.Status {
			dimErr := h.metricMeta.AddDimension(strings.ToLower(typ), strconv.FormatBool(state))
			if dimErr != nil {
				return dimErr
			}
		}

		// Send Metric
		_, err = h.dtClient.SendMetric(h.metricMeta)
		if err != nil {
			return err
		}
	}

	// if no err, returns nil...duh!
	return err
}

func (h *ManagedHandler) Monitor() (MonitorResult, error) {

	kindDimErr := h.metricMeta.AddDimension(KIND, h.metric.Spec.Kind)
	if kindDimErr != nil {
		return MonitorResult{}, fmt.Errorf("could not initialize '"+KIND+"' dimensions: %w", kindDimErr)
	}
	groupDimErr := h.metricMeta.AddDimension(GROUP, h.metric.Spec.Group)
	if groupDimErr != nil {
		return MonitorResult{}, fmt.Errorf("could not initialize '"+GROUP+"' dimensions: %w", groupDimErr)
	}
	versionDimErr := h.metricMeta.AddDimension(VERSION, h.metric.Spec.Version)
	if versionDimErr != nil {
		return MonitorResult{}, fmt.Errorf("could not initialize '"+VERSION+"' dimensions: %w", versionDimErr)
	}

	result := MonitorResult{}
	err := h.sendStatusBasedMetricValue()

	if err != nil {
		result.Error = err
		result.Phase = v1.PhaseFailed
		result.Reason = "SendMetricFailed"
		result.Message = fmt.Sprintf("failed to send metric value to data sink. %s", err.Error())
	} else {
		result.Phase = v1.PhaseActive
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

func (h *ManagedHandler) getResourcesStatus() ([]ClusterResourceStatus, error) {
	managedResources, err := h.getManagedResources()
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

func (h *ManagedHandler) getManagedResources() ([]Managed, error) {

	crds := &apiextensionsv1.CustomResourceDefinitionList{} // get ALL custom resource definitions
	if err := h.client.List(context.Background(), crds); err != nil {
		return nil, err
	}

	var resourceCRDs []apiextensionsv1.CustomResourceDefinition
	for _, crd := range crds.Items {
		if h.hasCategory("crossplane", crd) && h.hasCategory("managed", crd) { // filter previously acquired crds
			resourceCRDs = append(resourceCRDs, crd)
		}
	}

	var resources []unstructured.Unstructured
	for _, crd := range resourceCRDs {

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

			list, err := h.dCli.Resource(gvr).List(context.Background(), metav1.ListOptions{}) // gets resources from all the available crds
			if err != nil {
				return nil, fmt.Errorf("could not find any matching resources for metric with filter '%s'. %w", h.metric.GvkToString(), err)
			}

			if len(list.Items) > 0 {
				resources = append(resources, list.Items...)
			}
		}
	}

	var managedResources []Managed
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

type Managed struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Spec       Spec              `json:"spec"`
	Metadata   metav1.ObjectMeta `json:"metadata"`
	Status     Status            `json:"status"`
}

type Status struct {
	AtProvider map[string]any `json:"forProvider"`
	Conditions []Condition    `json:"conditions"`
}

type Condition struct {
	LastTransitionTime string `json:"lastTransitionTime"`
	Message            string `json:"message"`
	Reason             string `json:"reason"`
	Status             string `json:"status"`
	Type               string `json:"type"`
}

type Spec struct {
	ForProvider map[string]any `json:"forProvider"`
}

type ClusterResourceStatus struct {
	MangedResource Managed
	Status         map[string]bool
}

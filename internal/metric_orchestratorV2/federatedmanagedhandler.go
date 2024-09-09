package metric_orchestratorV2

import (
	"context"
	"fmt"
	"strconv"

	"github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1alpha1"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1beta1"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/internal/clientoptl"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	rcli "sigs.k8s.io/controller-runtime/pkg/client"
)

func NewFederatedManagedHandler(metric v1beta1.FederatedManagedMetric, qc QueryConfig, gaugeMetric *clientoptl.Metric) (*FederatedManagedHandler, error) {
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

type FederatedManagedHandler struct {
	client      rcli.Client
	dCli        dynamic.Interface
	discoClient discovery.DiscoveryInterface

	metric v1beta1.FederatedManagedMetric

	gauge       *clientoptl.Metric
	clusterName *string
}

func (h *FederatedManagedHandler) Monitor() (MonitorResult, error) {

	result := MonitorResult{}

	resources, err := h.getResourcesStatus()

	if err != nil {
		result.Error = err
		result.Phase = v1alpha1.PhaseFailed
		result.Reason = "ResourceNotFound"
		result.Message = fmt.Sprintf("could not find any matching federated managed resources for metric '%s'", h.metric.Spec.Name)
		return result, nil
	}

	var dimensions []v1beta1.Dimension

	// this is not right, we need to do a group by on the resources based on gvk

	//groups := lo.GroupBy(resources, func(r ClusterResourceStatus) string {
	//	return fmt.Sprintf("%s/%s", r.MangedResource.Kind, r.MangedResource.APIVersion)
	//})
	//
	//for _, group := range groups {
	//
	//}

	for _, cr := range resources {
		dp := clientoptl.NewDataPoint().
			AddDimension(CLUSTER, *h.clusterName).
			AddDimension(KIND, cr.MangedResource.Kind).
			AddDimension(APIVERSION, cr.MangedResource.APIVersion).
			AddDimension("UUID", string(cr.MangedResource.Metadata.UID)). // this has to be unique, otherwise all the tuples are the same and the metric is not recorded properly
			SetValue(int64(1))

		for fieldName, state := range cr.Status {
			dp.AddDimension(fieldName, strconv.FormatBool(state))
			dimensions = append(dimensions, v1beta1.Dimension{Name: fieldName, Value: strconv.FormatBool(state)})
		}

		err = h.gauge.RecordMetrics(dp)
		if err != nil {
			return MonitorResult{}, fmt.Errorf("could not record metric: %w", err)
		}

	}

	result.Phase = v1alpha1.PhaseActive
	result.Reason = "MonitoringActive"
	result.Message = fmt.Sprintf("metric is monitoring federated managed resources '%s'", h.metric.Name)

	if dimensions != nil {
		result.Observation = &v1beta1.MetricObservation{Timestamp: metav1.Now(), Dimensions: []v1beta1.Dimension{{Name: dimensions[0].Name, Value: strconv.Itoa(len(resources))}}}
	} else {
		result.Observation = &v1beta1.MetricObservation{Timestamp: metav1.Now()}
	}

	return result, nil

}

func (h *FederatedManagedHandler) getResourcesStatus() ([]ClusterResourceStatus, error) {
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

// is used to check if a resource from the cluster has a specific field
func (h *FederatedManagedHandler) hasCategory(category string, crd apiextensionsv1.CustomResourceDefinition) bool {
	for _, v := range crd.Spec.Names.Categories {
		if v == category {
			return true
		}
	}

	return false
}

func (h *FederatedManagedHandler) getManagedResources() ([]Managed, error) {

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
				return nil, fmt.Errorf("could not find any matching resources for metric '%s'. %w", h.metric.Name, err)
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

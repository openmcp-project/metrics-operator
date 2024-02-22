package handler

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	businessv1 "github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1"
	dynwrapper "github.tools.sap/cloud-orchestration/co-metrics-operator/internal/client"
	common "github.tools.sap/cloud-orchestration/co-metrics-operator/internal/common"
)

// This struct safes the clients and metrics it needs to communicate everything to the dynatrace backend via the client
type ManagedMetricHandler struct {
	dynaMetric         dynwrapper.MetricMetadata
	dynaClient         dynwrapper.DynatraceClient
	k8sClient          client.Client
	k8sMetric          businessv1.ManagedMetric
	k8sDynamic         dynamic.Interface
	recContext         context.Context
	recRequest         ctrl.Request
	managedResources   []Managed
	metricMetadataSent bool
}

// creates a new handler which than can handle metrics.
//
// Entry Point of the Handler: HandleGenericMetric()
//
// To successfully handle the metric, you need the following:
//
// - A Seceret with Token, Host and Path to your dynatrace enviroment
func NewMangedMetricHandler(ctx context.Context, req ctrl.Request, client client.Client, dCLient dynamic.Interface) (ManagedMetricHandler, error) {
	managed := ManagedMetricHandler{
		k8sClient:          client,
		recContext:         ctx,
		recRequest:         req,
		metricMetadataSent: false,
		k8sDynamic:         dCLient,
	}

	metric, err := managed.getClusterMetric()
	managed.k8sMetric = metric
	if err != nil {
		metric, err := managed.createClusterMetric()
		managed.k8sMetric = metric
		if err != nil {
			return ManagedMetricHandler{}, err
		}
	}

	secret, _ := common.GetClusterSecret(client, ctx)
	errMmgt := managed.getManagedResources()

	if errMmgt != nil {
		return ManagedMetricHandler{}, errMmgt
	}

	managed.dynaClient = dynwrapper.NewClient(string(secret.Data["Host"]), string(secret.Data["Path"]), string(secret.Data["Token"]))
	metricMetadata := dynwrapper.NewMetricMetadata(managed.k8sMetric.ObjectMeta.Name, managed.k8sMetric.Spec.Name, managed.k8sMetric.Spec.Description)

	kindDimErr := metricMetadata.AddDimension(KIND, managed.k8sMetric.Spec.Kind)
	if kindDimErr != nil {
		return ManagedMetricHandler{}, fmt.Errorf("could not initialize '"+KIND+"' dimensions: %w", kindDimErr)
	}
	groupDimErr := metricMetadata.AddDimension(GROUP, managed.k8sMetric.Spec.Group)
	if groupDimErr != nil {
		return ManagedMetricHandler{}, fmt.Errorf("could not initialize '"+GROUP+"' dimensions: %w", groupDimErr)
	}
	versionDimErr := metricMetadata.AddDimension(VERSION, managed.k8sMetric.Spec.Version)
	if versionDimErr != nil {
		return ManagedMetricHandler{}, fmt.Errorf("could not initialize '"+VERSION+"' dimensions: %w", versionDimErr)
	}

	managed.dynaMetric = metricMetadata

	return managed, nil
}

// Entry Point of the Handler, this is the only function call you need!
func (b *ManagedMetricHandler) HandleManagedMetric() (int, businessv1.ActivationType, error) {
	status, err := b.HandleClusterMetric()
	if err != nil || status == businessv1.ActivationDisabled {
		return -1, businessv1.ActivationDisabled, err
	}
	fmt.Printf("%s	INFO	Custom metric resource is handeld\n", time.Now().UTC().Format("2006-01-02T15:04:05+01:00"))

	status, err = b.SendStatusBasedMetric()
	if err != nil {
		return -1, businessv1.ActivationDisabled, err
	}

	err = b.updateCLusterMetricStatus(status)
	if err != nil {
		return -1, businessv1.ActivationDisabled, err
	}

	return b.k8sMetric.Spec.Frequency, status, nil
}

// creates an empty metric (will be removed, if no metric is present than one needs to be craeted through yaml) -> fix in future PR
func (b *ManagedMetricHandler) createClusterMetric() (businessv1.ManagedMetric, error) {
	managed := businessv1.ManagedMetric{}
	if err := b.k8sClient.Create(b.recContext, &managed); err != nil {
		return businessv1.ManagedMetric{}, err
	}

	return managed, nil
}

// Get the Metric from the cluster, which you deployed earlier into the system
//
// Deployment of Metric:
//
// you can deploy the metric through: kubectl apply -f sample/metric.yaml
func (b *ManagedMetricHandler) getClusterMetric() (businessv1.ManagedMetric, error) {
	managed := businessv1.ManagedMetric{}
	if err := b.k8sClient.Get(b.recContext, b.recRequest.NamespacedName, &managed); err != nil && errors.IsNotFound(err) {
		return managed, nil
	}

	return managed, nil
}

// gets the count of the resource specified
func (b *ManagedMetricHandler) getClusterResourceCount() (int, error) {
	list, err := b.k8sDynamic.Resource(
		schema.GroupVersionResource{
			Group:    b.k8sMetric.Spec.Group,
			Version:  b.k8sMetric.Spec.Version,
			Resource: b.k8sMetric.Spec.Kind,
		},
	).List(b.recContext, metav1.ListOptions{LabelSelector: b.k8sMetric.Spec.LabelSelector, FieldSelector: b.k8sMetric.Spec.FieldSelector})

	if err != nil {
		return 0, fmt.Errorf("could not find resources from metric: %w", err)
	}

	return len(list.Items), nil
}

// Queries all managed resources (crossplane based) and safes them inside of the struct for later use
func (b *ManagedMetricHandler) getManagedResources() error {
	crds := &apiextensionsv1.CustomResourceDefinitionList{} // get ALL cutsom resource definitions
	if err := b.k8sClient.List(b.recContext, crds); err != nil {
		return err
	}

	var resourceCRDs []apiextensionsv1.CustomResourceDefinition
	for _, crd := range crds.Items {
		if b.hasCategory("crossplane", crd) && b.hasCategory("managed", crd) { // filter previously acquired crds
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

			list, err := b.k8sDynamic.Resource(gvr).List(b.recContext, metav1.ListOptions{}) // gets resources from all the available crds
			if err != nil {
				return err
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
			return err
		}

		managedResources = append(managedResources, managed)
	}
	b.managedResources = managedResources
	return nil
}

// is used to check if a resource from the cluster has a specific field
func (b *ManagedMetricHandler) hasCategory(category string, crd apiextensionsv1.CustomResourceDefinition) bool {
	for _, v := range crd.Spec.Names.Categories {
		if v == category {
			return true
		}
	}

	return false
}

type ClusterResourceStatus struct {
	MangedResource Managed
	Status         map[string]bool
}

// Uses the managed resources from the cluster, to extract the status fields common for crossplane resources
//
// Ready: true | false
//
// Synced: true | false
func (b *ManagedMetricHandler) getClusterResourceStatus() ([]ClusterResourceStatus, error) {
	err := b.getManagedResources()
	if err != nil {
		return []ClusterResourceStatus{}, err
	}

	crStatuses := make([]ClusterResourceStatus, 0)

	for _, item := range b.managedResources {
		rsStatus := ClusterResourceStatus{MangedResource: item, Status: make(map[string]bool)}
		for _, condition := range item.Status.Conditions {
			status, _ := strconv.ParseBool(condition.Status)
			rsStatus.Status[condition.Type] = status
		}
		crStatuses = append(crStatuses, rsStatus)
	}

	return crStatuses, nil
}

// update the status of the resource on the cluster, so the cluster stays up to date
func (b *ManagedMetricHandler) updateCLusterMetricStatus(status businessv1.ActivationType) error {
	b.k8sMetric.Status.Active = status
	if err := b.k8sClient.Status().Update(b.recContext, &b.k8sMetric); err != nil {
		return err
	}
	return nil
}

// checks for and validates the current deloyed metric
func (b *ManagedMetricHandler) HandleClusterMetric() (businessv1.ActivationType, error) {
	status := businessv1.ActivationEnabled
	managed, err := b.getClusterMetric()
	if err != nil {
		managed, err = b.createClusterMetric()
		if err != nil {
			status = businessv1.ActivationDisabled
			return status, err
		}
	}

	if managed.Spec.Frequency < 1 {
		status = businessv1.ActivationDisabled
		return status, fmt.Errorf("no valid frequency")
	}

	b.k8sMetric = managed
	fmt.Printf("%s	INFO	Cluster metric handled: %s \n", time.Now().UTC().Format("2006-01-02T15:04:05+01:00"), managed.Spec.Name)
	return status, nil
}

// Sends the metric with the status of crossplane resources defined as dimensions count of the defined resource
func (b *ManagedMetricHandler) SendStatusBasedMetric() (businessv1.ActivationType, error) {
	// add the Datapoint
	b.dynaMetric.AddDatapoint(1)
	resources, err := b.getClusterResourceStatus()
	if err != nil {
		return businessv1.ActivationDisabled, err
	}

	for _, cr := range resources {
		b.dynaMetric.ClearDimensions()
		_ = b.dynaMetric.AddDimension("kind", cr.MangedResource.Kind)
		_ = b.dynaMetric.AddDimension("apiVersion", cr.MangedResource.APIVersion)

		// TODO: add mcp name as well later
		// b.dynaMetric.AddDimension("name", ...)

		for typ, state := range cr.Status {
			dimErr := b.dynaMetric.AddDimension(strings.ToLower(typ), strconv.FormatBool(state))
			if dimErr != nil {
				return businessv1.ActivationDisabled, dimErr
			}
		}

		// Send Metric
		_, err = b.dynaClient.SendMetric(b.dynaMetric)
		if err != nil {
			return businessv1.ActivationDisabled, err
		}
		fmt.Printf("%s	INFO	Metric %s with status is sent\n", time.Now().UTC().Format("2006-01-02T15:04:05+01:00"), cr.MangedResource.Metadata.Name)
	}

	return businessv1.ActivationEnabled, nil
}

// Sends the metric with the instance count of the defined resource
func (b *ManagedMetricHandler) SendValueBasedMetric() (businessv1.ActivationType, error) {
	count, err := b.getClusterResourceCount()
	if err != nil {
		return businessv1.ActivationDisabled, err
	}

	// add the Datapoint
	b.dynaMetric.AddDatapoint(float64(count))
	// Send Metric
	_, err = b.dynaClient.SendMetric(b.dynaMetric)
	if err != nil {
		return businessv1.ActivationDisabled, err
	}

	return businessv1.ActivationEnabled, nil
}

// Sends the metadata of the metric
// this only works when sending the metadata the first time, so please track if you have sent it already
func (b *ManagedMetricHandler) SendMetadata() (businessv1.ActivationType, *http.Response, error) {
	_, res, err := b.dynaClient.SendMetricMetadata(b.dynaMetric)
	if err != nil {
		return businessv1.ActivationDisabled, &http.Response{}, err
	}

	return businessv1.ActivationEnabled, res, nil
}

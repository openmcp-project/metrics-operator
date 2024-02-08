package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client"

	businessv1 "github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1"
	dynwrapper "github.tools.sap/cloud-orchestration/co-metrics-operator/internal/client"
	common "github.tools.sap/cloud-orchestration/co-metrics-operator/internal/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	KIND    string = "kind"
	GROUP   string = "group"
	VERSION string = "version"
)

// This struct safes the clients and metrics it needs to communicate everything to the dynatrace backend via the client
type GenericMetricHandler struct {
	dynaMetric         dynwrapper.MetricMetadata
	dynaClient         dynwrapper.DynatraceClient
	k8sClient          client.Client
	k8sMetric          businessv1.Metric
	k8sDynamic         dynamic.Interface
	reccontext         context.Context
	recrequest         ctrl.Request
	MetricMetadataSent bool
}

// creates a new handler which than can handle metrics.
//
// Entry Point of the Handler: HandleGenericMetric()
//
// To successfully handle the metric, you need the following:
//
// - A Seceret with Token, Host and Path to your dynatrace enviroment
func NewGenericMetricHandler(ctx context.Context, req ctrl.Request, client client.Client, dClient dynamic.Interface) (GenericMetricHandler, error) {
	generic := GenericMetricHandler{
		k8sClient:  client,
		reccontext: ctx,
		recrequest: req,
		k8sDynamic: dClient,
	}

	metric, err := generic.getClusterMetric()
	generic.k8sMetric = metric
	if err != nil {
		metric, err := generic.createClusterMetric()
		generic.k8sMetric = metric
		if err != nil {
			return GenericMetricHandler{}, err
		}
	}

	secret, _ := common.GetClusterSecret(client, ctx)
	generic.dynaClient = dynwrapper.NewClient(string(secret.Data["Host"]), string(secret.Data["Path"]), string(secret.Data["Token"])) // needs to be set here because the secret needs the predefined object
	metricMetadata := dynwrapper.NewMetricMetadata(generic.k8sMetric.ObjectMeta.Name, generic.k8sMetric.Spec.Name, generic.k8sMetric.Spec.Description)

	kindDimErr := metricMetadata.AddDimension(KIND, generic.k8sMetric.Spec.Kind)
	if kindDimErr != nil {
		return GenericMetricHandler{}, fmt.Errorf("could not initialize '"+KIND+"' dimensions: %w", kindDimErr)
	}
	groupDimErr := metricMetadata.AddDimension(GROUP, generic.k8sMetric.Spec.Group)
	if groupDimErr != nil {
		return GenericMetricHandler{}, fmt.Errorf("could not initialize '"+GROUP+"' dimensions: %w", groupDimErr)
	}
	versionDimErr := metricMetadata.AddDimension(VERSION, generic.k8sMetric.Spec.Version)
	if versionDimErr != nil {
		return GenericMetricHandler{}, fmt.Errorf("could not initialize '"+VERSION+"' dimensions: %w", versionDimErr)
	}

	generic.dynaMetric = metricMetadata

	return generic, nil
}

// Entry Point of the Handler, this is the only function call you need!
func (b *GenericMetricHandler) HandleGenericMetric() (int, businessv1.ActivationType, error) {
	status, err := b.HandleClusterMetric()
	if err != nil || status == businessv1.ActivationDisabled {
		return -1, status, err
	}

	status, err = b.SendValueBasedMetric()
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
func (b *GenericMetricHandler) createClusterMetric() (businessv1.Metric, error) {
	generic := businessv1.Metric{}
	if err := b.k8sClient.Create(b.reccontext, &generic); err != nil {
		return businessv1.Metric{}, err
	}

	return generic, nil
}

// Get the Metric from the cluster, which you deployed earlier into the system
//
// Deployment of Metric:
//
// you can deploy the metric through: kubectl apply -f sample/metric.yaml
func (b *GenericMetricHandler) getClusterMetric() (businessv1.Metric, error) {
	generic := businessv1.Metric{}
	if err := b.k8sClient.Get(b.reccontext, b.recrequest.NamespacedName, &generic); err != nil && errors.IsNotFound(err) {
		return generic, nil
	}

	return generic, nil
}

// gets the count of the resource specified in the metric deployed in the cluster
func (b *GenericMetricHandler) getClusterResourceCount() (int, error) {
	list, err := b.k8sDynamic.Resource(
		schema.GroupVersionResource{
			Group:    b.k8sMetric.Spec.Group,
			Version:  b.k8sMetric.Spec.Version,
			Resource: b.k8sMetric.Spec.Kind,
		},
	).List(b.reccontext, metav1.ListOptions{LabelSelector: b.k8sMetric.Spec.LabelSelector, FieldSelector: b.k8sMetric.Spec.FieldSelector})

	if err != nil {
		return 0, fmt.Errorf("could not find resources from metric: %w", err)
	}

	return len(list.Items), nil
}

// update the status of the resource on the cluster, so the cluster stays up to date
func (b *GenericMetricHandler) updateCLusterMetricStatus(status businessv1.ActivationType) error {
	b.k8sMetric.Status.Active = status
	if err := b.k8sClient.Status().Update(b.reccontext, &b.k8sMetric); err != nil {
		return err
	}
	return nil
}

// checks for and validates the current deloyed metric
func (b *GenericMetricHandler) HandleClusterMetric() (businessv1.ActivationType, error) {
	status := businessv1.ActivationEnabled
	generic, err := b.getClusterMetric()
	if err != nil {
		generic, err = b.createClusterMetric()
		if err != nil {
			status = businessv1.ActivationDisabled
			return status, err
		}
	}

	if generic.Spec.Frequency < 1 {
		status = businessv1.ActivationDisabled
		return status, fmt.Errorf("no valid frequency")
	}

	b.k8sMetric = generic
	fmt.Printf("%s	INFO	Cluster metric handled: %s \n", time.Now().UTC().Format("2006-01-02T15:04:05+01:00"), generic.Spec.Name)
	return status, nil
}

// Sends the metric with the instance count of the defined resource
func (b *GenericMetricHandler) SendValueBasedMetric() (businessv1.ActivationType, error) {
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
func (b *GenericMetricHandler) SendMetadata() (businessv1.ActivationType, *http.Response, error) {
	_, res, err := b.dynaClient.SendMetricMetadata(b.dynaMetric)
	if err != nil {
		return businessv1.ActivationDisabled, &http.Response{}, err
	}

	return businessv1.ActivationEnabled, res, nil
}

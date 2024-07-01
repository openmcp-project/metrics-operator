/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	businessv1 "github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/internal/common"
	orc "github.tools.sap/cloud-orchestration/co-metrics-operator/internal/metric_orchestratorV2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	RequeueAfterError = 2
)

func NewMetricReconciler(mgr ctrl.Manager) *MetricReconciler {
	return &MetricReconciler{
		inClient:   mgr.GetClient(),
		RestConfig: mgr.GetConfig(),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("metrics-controller"),
	}
}

func (r *MetricReconciler) GetClient() client.Client {
	return r.inClient
}

func (r *MetricReconciler) GetRestConfig() *rest.Config {
	return r.RestConfig
}

// MetricReconciler reconciles a Metric object
type MetricReconciler struct {
	// Internal client to K8S API. K8S cluster where the operator runs.
	inClient   client.Client
	RestConfig *rest.Config
	Scheme     *runtime.Scheme

	Recorder record.EventRecorder
}

const (
	kubeconfigKey = "kubeconfig"
)

// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=business.orchestrate.cloud.sap,resources=metrics,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=business.orchestrate.cloud.sap,resources=metrics/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=business.orchestrate.cloud.sap,resources=metrics/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *MetricReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var l = log.FromContext(ctx)

	/*
			1. Load the generic metric using the client
		 	All method should take the context to allow for cancellation (like CancellationToken)
	*/
	metric := businessv1.Metric{}
	if errLoad := r.GetClient().Get(ctx, req.NamespacedName, &metric); errLoad != nil {
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can also get them
		// on delete requests.
		if apierrors.IsNotFound(errLoad) {
			l.Info("Generic Metric not found")
			return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, nil
		}
		l.Error(errLoad, "unable to fetch Generic Metric")
		return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, errLoad
	}

	/*
		1.1 Get the Secret that holds the Dynatrace credentials
	*/
	secret, errSecret := common.GetCredentialsSecret(r.GetClient(), ctx)
	if errSecret != nil {
		l.Error(errSecret, fmt.Sprintf("unable to fetch Secret '%s' in namespace '%s' that stores the credentials to Data Sink", common.SecretName, common.SecretNameSpace))
		r.Recorder.Event(&metric, "Error", "SecretNotFound", fmt.Sprintf("unable to fetch Secret '%s' in namespace '%s' that stores the credentials to Data Sink", common.SecretName, common.SecretNameSpace))
		return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, errSecret
	}

	credentials := common.GetCredentialData(secret)

	/*
		1.2 Create QueryConfig to query the resources in the K8S cluster or external cluster based on the kubeconfig secret reference
	*/
	queryConfig, err := createQueryConfig(ctx, metric.Spec.KubeConfigSecretRef, r)
	if err != nil {
		return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, err
	}

	/*
		2. Create a new orchestrator
	*/
	orchestrator, errOrch := orc.NewOrchestrator(credentials, queryConfig).WithGeneric(metric)
	if errOrch != nil {
		l.Error(errOrch, "unable to create generic metric orchestrator monitor")
		r.Recorder.Event(&metric, "Warning", "OrchestratorCreation", "unable to create orchestrator")
		return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, errOrch
	}

	result, errMon := orchestrator.Handler.Monitor()

	if errMon != nil {
		l.Error(errMon, fmt.Sprintf("generic metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, errMon
	}

	/*
		3. Update the status of the metric with conditions and phase
	*/
	switch result.Phase {
	case businessv1.PhaseActive:
		metric.SetConditions(common.Available(result.Message))
		r.Recorder.Event(&metric, "Normal", "MetricAvailable", result.Message)
	case businessv1.PhaseFailed:
		l.Error(result.Error, result.Message, "reason", result.Reason)
		metric.SetConditions(common.Error(result.Message))
		r.Recorder.Event(&metric, "Warning", "MetricFailed", result.Message)
	case businessv1.PhasePending:
		metric.SetConditions(common.Creating())
		r.Recorder.Event(&metric, "Normal", "MetricPending", result.Message)
	}

	metric.Status.Phase = result.Phase

	// conditions are not persisted until the status is updated
	errUp := r.GetClient().Status().Update(ctx, &metric)
	if errUp != nil {
		l.Error(errMon, fmt.Sprintf("generic metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, errUp
	}

	/*
		4. Requeue the metric after the frequency or after 2 minutes if an error occurred
	*/
	var requeueTime int
	if result.Error != nil {
		requeueTime = RequeueAfterError
	} else {
		requeueTime = metric.Spec.Frequency
	}

	l.Info(fmt.Sprintf("generic metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, requeueTime))

	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: time.Duration(requeueTime) * time.Minute,
	}, nil
}

func createQueryConfig(ctx context.Context, kcRef *businessv1.KubeConfigSecretRef, r InsightReconciler) (orc.QueryConfig, error) {
	var queryConfig orc.QueryConfig
	// Kubernetes client to the external cluster if defined
	if kcRef != nil {
		qc, err := createExternalQueryConfig(ctx, kcRef, r.GetClient())
		if err != nil {
			return orc.QueryConfig{}, err
		}
		queryConfig = *qc
	} else {
		// local cluster name (where operator is deployed)
		clusterName, _ := getClusterInfo(r.GetRestConfig())
		queryConfig = orc.QueryConfig{Client: r.GetClient(), RestConfig: *r.GetRestConfig(), ClusterName: &clusterName}
	}
	return queryConfig, nil
}

func getClusterInfo(config *rest.Config) (string, error) {
	if config.Host == "" {
		return "", fmt.Errorf("config.Host is empty")
	}

	// Parse the host URL
	u, err := url.Parse(config.Host)
	if err != nil {
		return "", fmt.Errorf("failed to parse host URL: %v", err)
	}

	// Extract the hostname
	hostname := u.Hostname()

	// debugging only
	if hostname == "127.0.0.1" {
		return "localhost", nil
	}

	// Remove any prefix (like "kubernetes" or "kubernetes.default.svc")
	parts := strings.Split(hostname, ".")
	clusterName := parts[0]

	return clusterName, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MetricReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&businessv1.Metric{}).
		Complete(r)
}

func createExternalQueryConfig(ctx context.Context, kcRef *businessv1.KubeConfigSecretRef, inClient client.Client) (*orc.QueryConfig, error) {
	var secretName = kcRef.Name
	var secretNamespace = kcRef.Namespace

	// Retrieve the Secret
	secret := &corev1.Secret{}
	err := inClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret)
	if err != nil {
		errSecret := fmt.Errorf("failed to retrieve KubeConfig Secret Ref with name %s in namespace %s: %v", secretName, secretNamespace, err)
		return nil, errSecret
	}

	kubeconfigData, ok := secret.Data[kubeconfigKey]
	if !ok {
		return nil, fmt.Errorf("kubeconfig key %s not found in Secret", kubeconfigKey)
	}

	// Create a config from the kubeconfig data
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)

	if err != nil {
		return nil, fmt.Errorf("failed to create config from kubeconfig: %v", err)
	}

	kubeconfig, errKC := clientcmd.Load(kubeconfigData)
	if errKC != nil {
		return nil, fmt.Errorf("failed to load Config object from kubeconfigData: %v", errKC)
	}

	clusterName := kubeconfig.Contexts[kubeconfig.CurrentContext].Cluster

	// Create the client
	externalClient, err := client.New(config, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	return &orc.QueryConfig{Client: externalClient, RestConfig: *config, ClusterName: &clusterName}, nil
}

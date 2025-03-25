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
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	beta1 "github.com/SAP/metrics-operator/api/v1beta1"
	"github.com/SAP/metrics-operator/internal/clientoptl"
	"github.com/SAP/metrics-operator/internal/common"
	"github.com/SAP/metrics-operator/internal/config"
	orc "github.com/SAP/metrics-operator/internal/orchestrator"
)

// NewFederatedManagedMetricReconciler creates a new FederatedManagedMetricReconciler
func NewFederatedManagedMetricReconciler(mgr ctrl.Manager) *FederatedManagedMetricReconciler {
	return &FederatedManagedMetricReconciler{
		log: mgr.GetLogger().WithName("controllers").WithName("FederatedManagedMetric"),

		inCli:      mgr.GetClient(),
		RestConfig: mgr.GetConfig(),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("federated-managed-controller"),
	}
}

// FederatedManagedMetricReconciler reconciles a FederatedManagedMetric object
type FederatedManagedMetricReconciler struct {
	log logr.Logger

	inCli      client.Client
	Scheme     *runtime.Scheme
	RestConfig *rest.Config
	Recorder   record.EventRecorder
}

func (r *FederatedManagedMetricReconciler) getClient() client.Client {
	return r.inCli
}

func (r *FederatedManagedMetricReconciler) getRestConfig() *rest.Config {
	return r.RestConfig
}

func (r *FederatedManagedMetricReconciler) handleGetError(err error, log logr.Logger) (ctrl.Result, error) {
	// We'll ignore not-found errors. They can't be fixed by an immediate requeue.
	// We'll need to wait for a new notification. We can also get them on delete requests.
	if apierrors.IsNotFound(err) {
		log.Info("FederatedManagedMetric not found")
		return ctrl.Result{RequeueAfter: RequeueAfterError}, nil
	}
	log.Error(err, "Unable to fetch FederatedManagedMetric")
	return ctrl.Result{RequeueAfter: RequeueAfterError}, err
}

func (r *FederatedManagedMetricReconciler) scheduleNextReconciliation(metric *beta1.FederatedManagedMetric) (ctrl.Result, error) {

	elapsed := time.Since(metric.Status.LastReconcileTime.Time)
	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: metric.Spec.CheckInterval.Duration - elapsed,
	}, nil
}

func (r *FederatedManagedMetricReconciler) shouldReconcile(metric *beta1.FederatedManagedMetric) bool {
	if metric.Status.LastReconcileTime == nil {
		return true
	}
	elapsed := time.Since(metric.Status.LastReconcileTime.Time)
	return elapsed >= metric.Spec.CheckInterval.Duration
}

// Reconcile reads that state of the cluster for a FederatedManagedMetric object
// +kubebuilder:rbac:groups=metrics.cloud.sap,resources=federatedmanagedmetrics,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metrics.cloud.sap,resources=federatedmanagedmetrics/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metrics.cloud.sap,resources=federatedmanagedmetrics/finalizers,verbs=update
//
//nolint:gocyclo
func (r *FederatedManagedMetricReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := r.log.WithValues("namespace", req.NamespacedName, "name", req.Name)

	l.Info("Reconciling FederatedManagedMetric")

	l.Info(time.Now().String())

	/*
			1. Load the generic metric using the client
		 	All method should take the context to allow for cancellation (like CancellationToken)
	*/
	metric := beta1.FederatedManagedMetric{}
	if errLoad := r.getClient().Get(ctx, req.NamespacedName, &metric); errLoad != nil {
		return r.handleGetError(errLoad, l)
	}

	// Check if enough time has passed since the last reconciliation
	if !r.shouldReconcile(&metric) {
		return r.scheduleNextReconciliation(&metric)
	}

	/*
		1.1 Get the Secret that holds the Dynatrace credentials
	*/
	secret, errSecret := common.GetCredentialsSecret(ctx, r.getClient())
	if errSecret != nil {
		return r.handleSecretError(l, errSecret, metric)
	}

	credentials := common.GetCredentialData(secret)

	/*
		1.2 Create QueryConfig to query the resources in the K8S cluster or external cluster based on the kubeconfig secret reference
	*/
	queryConfigs, err := config.CreateExternalQueryConfigSet(ctx, metric.Spec.FederatedCARef, r.getClient(), r.getRestConfig())
	if err != nil {
		l.Error(err, "unable to create query configs")
		return ctrl.Result{RequeueAfter: RequeueAfterError}, err
	}

	metricClient, errCli := clientoptl.NewMetricClient(ctx, credentials.Host, credentials.Path, credentials.Token)

	if errCli != nil {
		l.Error(errCli, fmt.Sprintf("federated managed metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errCli
	}

	// should this be the group fo the gvr?
	metricClient.SetMeter("managed")

	gaugeMetric, errGauge := metricClient.NewMetric(metric.Name)
	if errGauge != nil {
		l.Error(errCli, fmt.Sprintf("federated metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errCli
	}

	for _, queryConfig := range queryConfigs {

		orchestrator, errOrch := orc.NewOrchestrator(credentials, queryConfig).WithFederatedManaged(metric, gaugeMetric)
		if errOrch != nil {
			l.Error(errOrch, "unable to create federate metric orchestrator monitor")
			r.Recorder.Event(&metric, "Warning", "OrchestratorCreation", "unable to create orchestrator")
			return ctrl.Result{RequeueAfter: RequeueAfterError}, errOrch
		}

		_, errMon := orchestrator.Handler.Monitor(ctx)

		if errMon != nil {
			l.Error(errMon, fmt.Sprintf("federated metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
			return ctrl.Result{RequeueAfter: RequeueAfterError}, errMon
		}

	}

	errExport := metricClient.ExportMetrics(ctx)
	if errExport != nil {
		metric.Status.Ready = "False"
		l.Error(errExport, fmt.Sprintf("federated managed metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
	} else {
		metric.Status.Ready = "True"
	}

	// Update LastReconcileTime
	now := metav1.Now()
	metric.Status.LastReconcileTime = &now

	// conditions are not persisted until the status is updated
	errUp := r.getClient().Status().Update(ctx, &metric)
	if errUp != nil {
		l.Error(errUp, fmt.Sprintf("federated managed metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errUp
	}

	/*
		4. Re-queue the metric after the frequency or 2 minutes if an error occurred
	*/
	var requeueTime = metric.Spec.CheckInterval.Duration

	l.Info(fmt.Sprintf("generic metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, requeueTime))

	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: requeueTime,
	}, nil
}

func (r *FederatedManagedMetricReconciler) handleSecretError(l logr.Logger, errSecret error, metric beta1.FederatedManagedMetric) (ctrl.Result, error) {
	l.Error(errSecret, fmt.Sprintf("unable to fetch secret '%s' in namespace '%s' that stores the credentials to data sink", common.SecretName, common.SecretNameSpace))
	r.Recorder.Event(&metric, "Error", "SecretNotFound", fmt.Sprintf("unable to fetch secret '%s' in namespace '%s' that stores the credentials to data sink", common.SecretName, common.SecretNameSpace))
	return ctrl.Result{RequeueAfter: RequeueAfterError}, errSecret
}

// SetupWithManager sets up the controller with the Manager.
func (r *FederatedManagedMetricReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&beta1.FederatedManagedMetric{}).
		Complete(r)
}

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
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"github.com/openmcp-project/metrics-operator/internal/clientoptl"
	"github.com/openmcp-project/metrics-operator/internal/common"
	"github.com/openmcp-project/metrics-operator/internal/config"
	orc "github.com/openmcp-project/metrics-operator/internal/orchestrator"
)

// NewFederatedManagedMetricReconciler creates a new FederatedManagedMetricReconciler
func NewFederatedManagedMetricReconciler(mgr ctrl.Manager) *FederatedManagedMetricReconciler {
	return &FederatedManagedMetricReconciler{
		log: mgr.GetLogger().WithName("controllers").WithName("FederatedManagedMetric"),

		inCli:      mgr.GetClient(),
		RestConfig: mgr.GetConfig(),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorder("federated-managed-controller"),
	}
}

// FederatedManagedMetricReconciler reconciles a FederatedManagedMetric object
type FederatedManagedMetricReconciler struct {
	log logr.Logger

	inCli      client.Client
	Scheme     *runtime.Scheme
	RestConfig *rest.Config
	Recorder   events.EventRecorder
}

func (r *FederatedManagedMetricReconciler) getClient() client.Client {
	return r.inCli
}

func (r *FederatedManagedMetricReconciler) getRestConfig() *rest.Config {
	return r.RestConfig
}

// getDataSinkCredentials fetches DataSink configuration and credentials
func (r *FederatedManagedMetricReconciler) getDataSinkCredentials(ctx context.Context, federatedManagedMetric *v1alpha1.FederatedManagedMetric, l logr.Logger) (common.DataSinkCredentials, error) {
	retriever := NewDataSinkCredentialsRetriever(r.getClient(), r.Recorder)
	return retriever.GetDataSinkCredentials(ctx, federatedManagedMetric.Spec.DataSinkRef, federatedManagedMetric, l)
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

func (r *FederatedManagedMetricReconciler) scheduleNextReconciliation(metric *v1alpha1.FederatedManagedMetric) ctrl.Result {

	elapsed := time.Since(metric.Status.LastReconcileTime.Time)
	return ctrl.Result{
		RequeueAfter: metric.Spec.Interval.Duration - elapsed,
	}
}

func (r *FederatedManagedMetricReconciler) shouldReconcile(metric *v1alpha1.FederatedManagedMetric) bool {
	if metric.Status.LastReconcileTime == nil {
		return true
	}
	elapsed := time.Since(metric.Status.LastReconcileTime.Time)
	return elapsed >= metric.Spec.Interval.Duration
}

// Reconcile reads that state of the cluster for a FederatedManagedMetric object
// +kubebuilder:rbac:groups=metrics.openmcp.cloud,resources=federatedmanagedmetrics,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=metrics.openmcp.cloud,resources=federatedmanagedmetrics/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=metrics.openmcp.cloud,resources=federatedmanagedmetrics/finalizers,verbs=update
// +kubebuilder:rbac:groups=metrics.openmcp.cloud,resources=datasinks,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get
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
	metric := v1alpha1.FederatedManagedMetric{}
	if errLoad := r.getClient().Get(ctx, req.NamespacedName, &metric); errLoad != nil {
		return r.handleGetError(errLoad, l)
	}

	// Defer status update to ensure it's always called
	defer func() {
		if err := r.getClient().Status().Update(ctx, &metric); err != nil {
			l.Error(err, "Failed to update FederatedManagedMetric status")
		}
	}()

	// Initialize Ready condition if not present
	if meta.FindStatusCondition(metric.Status.Conditions, v1alpha1.TypeReady) == nil {
		metric.SetConditions(common.ReadyUnknown("Reconciling", "Initial reconciliation"))
	}

	// Check if enough time has passed since the last reconciliation
	if !r.shouldReconcile(&metric) {
		return r.scheduleNextReconciliation(&metric), nil
	}

	/*
		1.1 Get the DataSink credentials
	*/
	credentials, errCredentials := r.getDataSinkCredentials(ctx, &metric, l)
	if errCredentials != nil {
		metric.SetConditions(common.ReadyFalse("DataSinkUnavailable", errCredentials.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errCredentials
	}

	/*
		1.2 Create QueryConfig to query the resources in the K8S cluster or external cluster based on the kubeconfig secret reference
	*/
	queryConfigs, err := config.CreateExternalQueryConfigSet(ctx, metric.Spec.FederatedClusterAccessRef, r.getClient(), r.getRestConfig(), config.CreateExternalQueryConfigSetOptions{})
	if err != nil {
		metric.SetConditions(common.ReadyFalse("QueryConfigCreationFailed", err.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		l.Error(err, "unable to create query configs")
		return ctrl.Result{RequeueAfter: RequeueAfterError}, err
	}

	metricClient, errCli := clientoptl.NewMetricClient(ctx, credentials.Host, credentials.Token)

	if errCli != nil {
		metric.SetConditions(common.ReadyFalse("OTLPClientCreationFailed", errCli.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		l.Error(errCli, fmt.Sprintf("federated managed metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errCli
	}

	defer func() {
		if err := metricClient.Close(ctx); err != nil {
			l.Error(err, "Failed to close metric client during federated managed metric reconciliation", "metric", metric.Name)
		}
	}()

	// should this be the group fo the gvr?
	metricClient.SetMeter("managed")

	gaugeMetric, errGauge := metricClient.NewMetric(metric.Name)
	if errGauge != nil {
		metric.SetConditions(common.ReadyFalse("MetricCreationFailed", errGauge.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		l.Error(errGauge, fmt.Sprintf("federated managed metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: RequeueAfterError}, errGauge
	}

	for _, queryConfig := range queryConfigs {

		orchestrator, errOrch := orc.NewOrchestrator(credentials, queryConfig).WithFederatedManaged(metric, gaugeMetric)
		if errOrch != nil {
			metric.SetConditions(common.ReadyFalse("OrchestratorCreationFailed", errOrch.Error()))
			metric.Status.Ready = v1alpha1.StatusStringFalse
			l.Error(errOrch, "unable to create federate metric orchestrator monitor")
			r.Recorder.Eventf(&metric, nil, "Warning", "OrchestratorCreation", "Reconcile", "unable to create orchestrator")
			return ctrl.Result{RequeueAfter: RequeueAfterError}, errOrch
		}

		_, errMon := orchestrator.Handler.Monitor(ctx)

		if errMon != nil {
			metric.SetConditions(common.ReadyFalse("MonitoringFailed", errMon.Error()))
			metric.Status.Ready = v1alpha1.StatusStringFalse
			l.Error(errMon, fmt.Sprintf("federated managed metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
			return ctrl.Result{RequeueAfter: RequeueAfterError}, errMon
		}

	}

	errExport := metricClient.ExportMetrics(ctx)
	if errExport != nil {
		metric.SetConditions(common.ReadyFalse("MetricExportFailed", errExport.Error()))
		metric.Status.Ready = v1alpha1.StatusStringFalse
		l.Error(errExport, fmt.Sprintf("federated managed metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
	} else {
		metric.SetConditions(common.ReadyTrue("Federated managed metric reconciled successfully"))
		metric.Status.Ready = v1alpha1.StatusStringTrue
	}

	// Update LastReconcileTime
	now := metav1.Now()
	metric.Status.LastReconcileTime = &now

	// Note: Status update is handled by the defer function at the beginning

	/*
		4. Re-queue the metric after the frequency or 2 minutes if an error occurred
	*/
	var requeueTime time.Duration
	if errExport != nil {
		requeueTime = RequeueAfterError
	} else {
		requeueTime = metric.Spec.Interval.Duration
	}

	l.Info(fmt.Sprintf("federated managed metric '%s' re-queued for execution in %v\n", metric.Spec.Name, requeueTime))

	return ctrl.Result{
		RequeueAfter: requeueTime,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *FederatedManagedMetricReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.FederatedManagedMetric{}).
		Complete(r)
}

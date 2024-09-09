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
	insight "github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1alpha1"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1beta1"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/internal/common"
	"github.tools.sap/cloud-orchestration/co-metrics-operator/internal/config"
	orc "github.tools.sap/cloud-orchestration/co-metrics-operator/internal/metric_orchestratorV2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewSingleMetricReconciler(mgr ctrl.Manager) *SingleMetricReconciler {
	return &SingleMetricReconciler{
		log: mgr.GetLogger().WithName("controllers").WithName("SingleMetric"),

		inCli:      mgr.GetClient(),
		RestConfig: mgr.GetConfig(),
		Scheme:     mgr.GetScheme(),
		Recorder:   mgr.GetEventRecorderFor("single-controller"),
	}
}

func (r *SingleMetricReconciler) GetClient() client.Client {
	return r.inCli
}

// SingleMetricReconciler reconciles a SingleMetric object
type SingleMetricReconciler struct {
	log logr.Logger

	inCli      client.Client
	Scheme     *runtime.Scheme
	RestConfig *rest.Config
	Recorder   record.EventRecorder
}

func (r *SingleMetricReconciler) GetRestConfig() *rest.Config {
	return r.RestConfig
}

func (r *SingleMetricReconciler) scheduleNextReconciliation(metric *v1beta1.SingleMetric) (ctrl.Result, error) {

	elapsed := time.Since(metric.Status.LastReconcileTime.Time)
	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: (time.Duration(metric.Spec.Frequency) * time.Minute) - elapsed,
	}, nil
}

func (r *SingleMetricReconciler) shouldReconcile(metric *v1beta1.SingleMetric) bool {
	if metric.Status.LastReconcileTime == nil {
		return true
	}
	elapsed := time.Since(metric.Status.LastReconcileTime.Time)
	return elapsed >= time.Duration(metric.Spec.Frequency)*time.Minute
}

func (r *SingleMetricReconciler) handleGetError(err error, log logr.Logger) (ctrl.Result, error) {
	// we'll ignore not-found errors, since they can't be fixed by an immediate
	// requeue (we'll need to wait for a new notification), and we can also get them
	// on delete requests.
	if apierrors.IsNotFound(err) {
		log.Info("SingleMetric not found")
		return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, nil
	}
	log.Error(err, "unable to fetch SingleMetric")
	return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, err
}

// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=insight.orchestrate.cloud.sap,resources=singlemetrics,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=insight.orchestrate.cloud.sap,resources=singlemetrics/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=insight.orchestrate.cloud.sap,resources=singlemetrics/finalizers,verbs=update

// Reconcile handles the reconciliation of a SingleMetric object
// A SingleMetric represents a single metric with 1 time series and fixed dimensions
func (r *SingleMetricReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := r.log.WithValues("namespaced name", req.NamespacedName)

	l.Info("Reconciling SingleMetric")

	/*
			1. Load the generic metric using the client
		 	All method should take the context to allow for cancellation (like CancellationToken)
	*/
	metric := v1beta1.SingleMetric{}
	if errLoad := r.GetClient().Get(ctx, req.NamespacedName, &metric); errLoad != nil {
		return r.handleGetError(errLoad, l)
	}

	// Check if enough time has passed since the last reconciliation
	if !r.shouldReconcile(&metric) {
		return r.scheduleNextReconciliation(&metric)
	}

	/*
		1.1 Get the Secret that holds the Dynatrace credentials
	*/
	secret, errSecret := common.GetCredentialsSecret(r.GetClient(), ctx)
	if errSecret != nil {
		l.Error(errSecret, fmt.Sprintf("unable to fetch secret '%s' in namespace '%s' that stores the credentials to data sink", common.SecretName, common.SecretNameSpace))
		r.Recorder.Event(&metric, "Error", "SecretNotFound", fmt.Sprintf("unable to fetch secret '%s' in namespace '%s' that stores the credentials to data sink", common.SecretName, common.SecretNameSpace))
		return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, errSecret
	}

	credentials := common.GetCredentialData(secret)

	/*
		1.2 Create QueryConfig to query the resources in the K8S cluster or external cluster based on the kubeconfig secret reference
	*/
	queryConfig, err := createQC(ctx, metric.Spec.ClusterAccessRef, r)
	if err != nil {
		return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, err
	}

	/*
		2. Create a new orchestrator
	*/
	orchestrator, errOrch := orc.NewOrchestrator(credentials, queryConfig).WithSingle(metric)
	if errOrch != nil {
		l.Error(errOrch, "unable to create single metric orchestrator monitor")
		r.Recorder.Event(&metric, "Warning", "OrchestratorCreation", "unable to create orchestrator")
		return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, errOrch
	}

	result, errMon := orchestrator.Handler.Monitor()

	if errMon != nil {
		l.Error(errMon, fmt.Sprintf("single metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, errMon
	}

	/*
		3. Update the status of the metric with conditions and phase
	*/
	switch result.Phase {
	case insight.PhaseActive:
		metric.SetConditions(common.Available(result.Message))
		r.Recorder.Event(&metric, "Normal", "MetricAvailable", result.Message)
	case insight.PhaseFailed:
		l.Error(result.Error, result.Message, "reason", result.Reason)
		metric.SetConditions(common.Error(result.Message))
		r.Recorder.Event(&metric, "Warning", "MetricFailed", result.Message)
	case insight.PhasePending:
		metric.SetConditions(common.Creating())
		r.Recorder.Event(&metric, "Normal", "MetricPending", result.Message)
	}

	metric.Status.Ready = boolToString(result.Phase == insight.PhaseActive)
	metric.Status.Observation = v1beta1.MetricObservation{Timestamp: result.Observation.GetTimestamp(), LatestValue: result.Observation.GetValue()}

	// Update LastReconcileTime
	now := metav1.Now()
	metric.Status.LastReconcileTime = &now

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

// SetupWithManager sets up the controller with the Manager.
func (r *SingleMetricReconciler) SetupWithManager(mgr ctrl.Manager) error {

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1beta1.SingleMetric{}).
		Complete(r)
}

func createQC(ctx context.Context, rcaRef *v1beta1.ClusterAccessRef, r InsightReconciler) (orc.QueryConfig, error) {
	var queryConfig orc.QueryConfig
	// Kubernetes client to the external cluster if defined
	if rcaRef != nil {
		qc, err := config.CreateExternalQC(ctx, rcaRef, r.GetClient())
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

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

	"github.tools.sap/cloud-orchestration/co-metrics-operator/internal/common"
	orc "github.tools.sap/cloud-orchestration/co-metrics-operator/internal/metric_orchestratorV2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ctrl "sigs.k8s.io/controller-runtime"

	businessv1 "github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1"
)

// ManagedMetricReconciler reconciles a ManagedMetric object
type ManagedMetricReconciler struct {
	Client     client.Client
	RestConfig *rest.Config
	Scheme     *runtime.Scheme

	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=business.orchestrate.cloud.sap,resources=managedmetrics,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=business.orchestrate.cloud.sap,resources=managedmetrics/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=business.orchestrate.cloud.sap,resources=managedmetrics/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *ManagedMetricReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var l = log.FromContext(ctx)

	/*
			1. Load the managed metric using the client
		 	All method should take the context to allow for cancellation (like CancellationToken)
	*/
	metric := businessv1.ManagedMetric{}
	if errLoad := r.Client.Get(ctx, req.NamespacedName, &metric); errLoad != nil {
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can also get them
		// on delete requests.
		if apierrors.IsNotFound(errLoad) {
			l.Info("Managed Metric not found")
			return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, nil
		}
		l.Error(errLoad, "unable to fetch Managed Metric")
		return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, errLoad
	}

	/*
		1.1 Get the Secret that holds the Dynatrace credentials
	*/
	secret, errSecret := common.GetCredentialsSecret(r.Client, ctx)
	if errSecret != nil {
		l.Error(errSecret, fmt.Sprintf("unable to fetch Secret '%s' in namespace '%s' that stores the credentials to Data Sink", common.SecretName, common.SecretNameSpace))
		r.Recorder.Event(&metric, "Error", "SecretNotFound", fmt.Sprintf("unable to fetch Secret '%s' in namespace '%s' that stores the credentials to Data Sink", common.SecretName, common.SecretNameSpace))
		return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, errSecret
	}

	credentials := common.GetCredentialData(secret)

	/*
		2. Create a new orchestrator
	*/
	orchestrator, errOrch := orc.NewOrchestrator(r.RestConfig, credentials, r.Client).WithManaged(metric)
	if errOrch != nil {
		l.Error(errOrch, "unable to create managed metric orchestrator monitor")
		r.Recorder.Event(&metric, "Warning", "OrchestratorCreation", "unable to create orchestrator")
		return ctrl.Result{RequeueAfter: RequeueAfterError * time.Minute}, errOrch
	}

	result, errMon := orchestrator.Handler.Monitor()

	if errMon != nil {
		l.Error(errMon, fmt.Sprintf("managed metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
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
	errUp := r.Client.Status().Update(ctx, &metric)
	if errUp != nil {
		l.Error(errUp, fmt.Sprintf("managed metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, RequeueAfterError))
		return ctrl.Result{RequeueAfter: 2 * time.Minute}, errUp
	}

	/*
		4. Requeue the metric after the frequency or after 2 minutes if an error occurred
	*/
	var requeueTime int
	if result.Error != nil {
		requeueTime = 2
	} else {
		requeueTime = metric.Spec.Frequency
	}

	l.Info(fmt.Sprintf("managed metric '%s' re-queued for execution in %v minutes\n", metric.Spec.Name, requeueTime))

	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: time.Duration(requeueTime) * time.Minute,
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedMetricReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&businessv1.ManagedMetric{}).
		Complete(r)
}

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

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ctrl "sigs.k8s.io/controller-runtime"

	businessv1 "github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1"
	orchestrator "github.tools.sap/cloud-orchestration/co-metrics-operator/internal/metric-orchestrator"
)

// ManagedMetricReconciler reconciles a ManagedMetric object
type ManagedMetricReconciler struct {
	client.Client
	RestConfig         *rest.Config
	Scheme             *runtime.Scheme
	DynamicClient      dynamic.Interface
	MetricOrchestrator orchestrator.MetricOrchestrator
}

//+kubebuilder:rbac:groups=business.orchestrate.cloud.sap,resources=managedmetrics,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=business.orchestrate.cloud.sap,resources=managedmetrics/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=business.orchestrate.cloud.sap,resources=managedmetrics/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *ManagedMetricReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	freq, err := r.handleManagedMetric(ctx, req)
	if err != nil {
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: time.Duration(1) * time.Minute,
		}, err
	}

	fmt.Printf("%s	INFO	Requeued for Execution in %v Minutes\n", time.Now().UTC().Format("2006-01-02T15:04:05+01:00"), freq)
	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: time.Duration(freq) * time.Minute,
	}, nil
}

func (r *ManagedMetricReconciler) handleManagedMetric(ctx context.Context, req ctrl.Request) (int, error) {
	var status businessv1.ActivationType
	var err error

	r.MetricOrchestrator, err = orchestrator.NewMetricOrchestrator(ctx, req, r.Client, r.RestConfig)
	if err != nil {
		return -1, err
	}

	frequency, status, err := r.MetricOrchestrator.OrchestrateManagedMetric()

	// check and return Metric
	if err != nil || status == businessv1.ActivationDisabled {
		return -1, err
	}

	return frequency, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedMetricReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&businessv1.ManagedMetric{}).
		Complete(r)
}

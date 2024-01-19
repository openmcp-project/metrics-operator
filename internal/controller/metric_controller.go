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

	dynclient "github.tools.sap/cloud-orchestration/co-metrics-operator/internal/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	businessv1 "github.tools.sap/cloud-orchestration/co-metrics-operator/api/v1"
)

const (
	cDynatraceUrl string = "https://apm.cf.eu10.hana.ondemand.com/e/089d8509-cd61-4dbf-a85b-0bdd12ee1f16/api/v2"
	// needs to be set
	cDynatraceApiToken string = "..."
)

// MetricReconciler reconciles a Metric object
type MetricReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	DynamicClient dynamic.Interface
}

//+kubebuilder:rbac:groups=business.orchestrate.cloud.sap,resources=metrics,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=business.orchestrate.cloud.sap,resources=metrics/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=business.orchestrate.cloud.sap,resources=metrics/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Metric object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *MetricReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	metric := businessv1.Metric{}
	err := r.Client.Get(ctx, req.NamespacedName, &metric)
	if err != nil {
		return ctrl.Result{}, err
	}

	//TODO: create based on referenced configuration CRD and its secret
	dynPocClient := dynclient.NewDynatraceClientPoc(cDynatraceUrl, cDynatraceApiToken)

	metricValue, err := r.collectResourcesByGroupVersionKind(ctx, metric.Spec.Group, metric.Spec.Version, metric.Spec.Kind)
	if err != nil {
		return ctrl.Result{}, err
	}

	dynPocClient.PostMetric(ctx, metric.Spec.Kind, metric.Spec.Group, metric.Spec.Version, metricValue)

	//TODO: needs to requeue
	return ctrl.Result{
		RequeueAfter: time.Duration(1) * time.Minute,
	}, nil
}

func (r *MetricReconciler) collectResourcesByGroupVersionKind(ctx context.Context, group string, version string, kind string) (int, error) {
	list, err := r.DynamicClient.Resource(
		schema.GroupVersionResource{
			Group:    group,
			Version:  version,
			Resource: kind,
		},
	).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("Could not find resources from metric")
	}
	return len(list.Items), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MetricReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&businessv1.Metric{}).
		Complete(r)
}

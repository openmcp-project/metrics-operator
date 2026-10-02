package controller

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
)

const reservedMetricNameForControllerTest = "controller_runtime_reconcile_time_seconds_count"

func TestReconcileRejectsReservedMetricNames(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	now := metav1.Now()
	tests := []struct {
		name      string
		object    client.Object
		reconcile func(client.Client) (ctrl.Result, error)
		fetch     func(client.Client) (client.Object, error)
	}{
		{
			name: "Metric",
			object: &v1alpha1.Metric{
				ObjectMeta: metav1.ObjectMeta{Name: "reserved", Namespace: "metrics-test"},
				Spec:       v1alpha1.MetricSpec{Name: reservedMetricNameForControllerTest, Interval: metav1.Duration{Duration: time.Hour}},
				Status:     v1alpha1.MetricStatus{Observation: v1alpha1.MetricObservation{Timestamp: now, LatestValue: "previous"}},
			},
			reconcile: func(c client.Client) (ctrl.Result, error) {
				return (&MetricReconciler{inCli: c, log: logr.Discard()}).Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "metrics-test", Name: "reserved"}})
			},
			fetch: func(c client.Client) (client.Object, error) {
				obj := &v1alpha1.Metric{}
				err := c.Get(ctx, types.NamespacedName{Namespace: "metrics-test", Name: "reserved"}, obj)
				return obj, err
			},
		},
		{
			name: "ManagedMetric",
			object: &v1alpha1.ManagedMetric{
				ObjectMeta: metav1.ObjectMeta{Name: "reserved", Namespace: "metrics-test"},
				Spec:       v1alpha1.ManagedMetricSpec{Name: reservedMetricNameForControllerTest, Interval: metav1.Duration{Duration: time.Hour}},
				Status:     v1alpha1.ManagedMetricStatus{Observation: v1alpha1.ManagedObservation{Timestamp: now}},
			},
			reconcile: func(c client.Client) (ctrl.Result, error) {
				return (&ManagedMetricReconciler{inClient: c}).Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "metrics-test", Name: "reserved"}})
			},
			fetch: func(c client.Client) (client.Object, error) {
				obj := &v1alpha1.ManagedMetric{}
				err := c.Get(ctx, types.NamespacedName{Namespace: "metrics-test", Name: "reserved"}, obj)
				return obj, err
			},
		},
		{
			name: "FederatedMetric",
			object: &v1alpha1.FederatedMetric{
				ObjectMeta: metav1.ObjectMeta{Name: "reserved", Namespace: "metrics-test"},
				Spec:       v1alpha1.FederatedMetricSpec{Name: reservedMetricNameForControllerTest, Interval: metav1.Duration{Duration: time.Hour}},
				Status:     v1alpha1.FederatedMetricStatus{LastReconcileTime: &now},
			},
			reconcile: func(c client.Client) (ctrl.Result, error) {
				return (&FederatedMetricReconciler{inCli: c, log: logr.Discard()}).Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "metrics-test", Name: "reserved"}})
			},
			fetch: func(c client.Client) (client.Object, error) {
				obj := &v1alpha1.FederatedMetric{}
				err := c.Get(ctx, types.NamespacedName{Namespace: "metrics-test", Name: "reserved"}, obj)
				return obj, err
			},
		},
		{
			name: "FederatedManagedMetric",
			object: &v1alpha1.FederatedManagedMetric{
				ObjectMeta: metav1.ObjectMeta{Name: "reserved", Namespace: "metrics-test"},
				Spec:       v1alpha1.FederatedManagedMetricSpec{Name: reservedMetricNameForControllerTest, Interval: metav1.Duration{Duration: time.Hour}},
				Status:     v1alpha1.FederatedManagedMetricStatus{LastReconcileTime: &now},
			},
			reconcile: func(c client.Client) (ctrl.Result, error) {
				return (&FederatedManagedMetricReconciler{inCli: c, log: logr.Discard()}).Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "metrics-test", Name: "reserved"}})
			},
			fetch: func(c client.Client) (client.Object, error) {
				obj := &v1alpha1.FederatedManagedMetric{}
				err := c.Get(ctx, types.NamespacedName{Namespace: "metrics-test", Name: "reserved"}, obj)
				return obj, err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(tc.object).WithObjects(tc.object).Build()
			_, err := tc.reconcile(c)
			require.Error(t, err)
			require.Contains(t, err.Error(), reservedMetricNameForControllerTest)

			updated, err := tc.fetch(c)
			require.NoError(t, err)
			condition := meta.FindStatusCondition(conditionsForMetricObject(updated), v1alpha1.TypeReady)
			require.NotNil(t, condition)
			require.Equal(t, metav1.ConditionFalse, condition.Status)
			require.Equal(t, "InvalidMetricName", condition.Reason)
			require.Equal(t, v1alpha1.StatusStringFalse, readinessForMetricObject(updated))
			require.Contains(t, condition.Message, reservedMetricNameForControllerTest)
		})
	}
}

func conditionsForMetricObject(object client.Object) []metav1.Condition {
	switch metric := object.(type) {
	case *v1alpha1.Metric:
		return metric.Status.Conditions
	case *v1alpha1.ManagedMetric:
		return metric.Status.Conditions
	case *v1alpha1.FederatedMetric:
		return metric.Status.Conditions
	case *v1alpha1.FederatedManagedMetric:
		return metric.Status.Conditions
	default:
		return nil
	}
}

func readinessForMetricObject(object client.Object) string {
	switch metric := object.(type) {
	case *v1alpha1.Metric:
		return metric.Status.Ready
	case *v1alpha1.ManagedMetric:
		return metric.Status.Ready
	case *v1alpha1.FederatedMetric:
		return metric.Status.Ready
	case *v1alpha1.FederatedManagedMetric:
		return metric.Status.Ready
	default:
		return ""
	}
}

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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
)

func TestPrometheusSeriesFinalizerLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	metric := &v1alpha1.Metric{ObjectMeta: metav1.ObjectMeta{
		Name:      "test-metric",
		Namespace: "test-namespace",
		UID:       types.UID("metric-uid"),
	}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(metric).Build()

	added, err := ensurePrometheusSeriesFinalizer(ctx, client, metric)
	require.NoError(t, err)
	require.True(t, added)
	require.True(t, controllerutil.ContainsFinalizer(metric, prometheusSeriesFinalizer))

	now := metav1.NewTime(time.Now())
	deletingMetric := &v1alpha1.Metric{ObjectMeta: metav1.ObjectMeta{
		Name:              "deleting-metric",
		Namespace:         "test-namespace",
		UID:               types.UID("deleting-metric-uid"),
		DeletionTimestamp: &now,
		Finalizers:        []string{prometheusSeriesFinalizer},
	}}
	deletingClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deletingMetric).Build()
	removed, err := removePrometheusSeriesOnDeletion(ctx, deletingClient, deletingMetric)
	require.NoError(t, err)
	require.True(t, removed)
	require.False(t, controllerutil.ContainsFinalizer(deletingMetric, prometheusSeriesFinalizer))
}

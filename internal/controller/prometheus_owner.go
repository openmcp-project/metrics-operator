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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	internalmetrics "github.com/openmcp-project/metrics-operator/internal/metrics"
)

const prometheusSeriesFinalizer = "metrics.open-control-plane.io/prometheus-series"

func ensurePrometheusSeriesFinalizer(ctx context.Context, c client.Client, metric client.Object) (bool, error) {
	if controllerutil.ContainsFinalizer(metric, prometheusSeriesFinalizer) {
		return false, nil
	}
	controllerutil.AddFinalizer(metric, prometheusSeriesFinalizer)
	return true, c.Update(ctx, metric)
}

func removePrometheusSeriesOnDeletion(ctx context.Context, c client.Client, metric client.Object) (bool, error) {
	if metric.GetDeletionTimestamp().IsZero() {
		return false, nil
	}
	internalmetrics.RemoveMetricOwner(metric.GetUID())
	if !controllerutil.ContainsFinalizer(metric, prometheusSeriesFinalizer) {
		return true, nil
	}
	controllerutil.RemoveFinalizer(metric, prometheusSeriesFinalizer)
	return true, c.Update(ctx, metric)
}

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

package metrics

import (
	"fmt"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isolatedCollector(t *testing.T) (*dynamicGaugeCollector, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	collector := newDynamicGaugeCollector(reg)
	return collector, reg
}

func record(t *testing.T, collector *dynamicGaugeCollector, metricName, namespace string, dims map[string]string, value int64) {
	t.Helper()
	require.NoError(t, collector.recordDataPoint(metricName, namespace, dims, value))
}

// gatherGauge returns all dto.Metric samples for the named metric family.
func gatherGauge(t *testing.T, reg prometheus.Gatherer, name string) []*dto.Metric {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() == name {
			return family.GetMetric()
		}
	}
	return nil
}

func labelValue(metric *dto.Metric, name string) string {
	for _, label := range metric.GetLabel() {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}

func TestRecordDataPoint_ProjectionLabels(t *testing.T) {
	collector, reg := isolatedCollector(t)
	dims := map[string]string{
		"schedule_name":  "crate",
		"namespace":      "velero",
		"schedule_phase": "Enabled",
		"schedule_cron":  "0 0/8 * * *",
		"cluster":        "prod-cluster",
	}
	record(t, collector, "velero_schedule_last_backup_seconds", "operator", dims, 1790755226)

	samples := gatherGauge(t, reg, "velero_schedule_last_backup_seconds")
	require.Len(t, samples, 1)
	metric := samples[0]
	assert.Equal(t, "crate", labelValue(metric, "schedule_name"))
	assert.Equal(t, "velero", labelValue(metric, "resource_namespace"))
	assert.Equal(t, "operator", labelValue(metric, "namespace"))
	assert.Equal(t, "Enabled", labelValue(metric, "schedule_phase"))
	assert.Equal(t, "0 0/8 * * *", labelValue(metric, "schedule_cron"))
	assert.Equal(t, "prod-cluster", labelValue(metric, "cluster"))
	assert.InDelta(t, 1790755226.0, metric.GetGauge().GetValue(), 0.001)
	assert.Equal(t, "velero", dims["namespace"], "recording must not mutate caller dimensions")
}

func TestRecordDataPoint_UpdatesAndMultipleMetricNames(t *testing.T) {
	collector, reg := isolatedCollector(t)
	record(t, collector, "metric_a", "ns", map[string]string{"x": "1"}, 10)
	record(t, collector, "metric_a", "ns", map[string]string{"x": "1"}, 15)
	record(t, collector, "metric_b", "ns", map[string]string{"x": "2"}, 20)

	samplesA := gatherGauge(t, reg, "metric_a")
	samplesB := gatherGauge(t, reg, "metric_b")
	require.Len(t, samplesA, 1)
	require.Len(t, samplesB, 1)
	assert.InDelta(t, 15.0, samplesA[0].GetGauge().GetValue(), 0.001)
	assert.InDelta(t, 20.0, samplesB[0].GetGauge().GetValue(), 0.001)
}

func TestRecordDataPoint_ProjectionSchemas(t *testing.T) {
	collector, reg := isolatedCollector(t)
	record(t, collector, "schema_metric", "first", map[string]string{"required": "one", "optional": "old"}, 1)
	record(t, collector, "schema_metric", "second", map[string]string{"required": "two"}, 2)
	record(t, collector, "schema_metric", "first", map[string]string{"required": "one", "new_optional": "later"}, 3)

	samples := gatherGauge(t, reg, "schema_metric")
	require.Len(t, samples, 3)
	var hasOldSchema, hasOmittedSchema, hasLaterSchema bool
	for _, metric := range samples {
		switch {
		case labelValue(metric, "optional") == "old":
			hasOldSchema = true
			assert.Equal(t, "first", labelValue(metric, "namespace"))
			assert.InDelta(t, 1.0, metric.GetGauge().GetValue(), 0.001)
		case labelValue(metric, "required") == "two":
			hasOmittedSchema = true
			assert.Equal(t, "second", labelValue(metric, "namespace"))
			assert.InDelta(t, 2.0, metric.GetGauge().GetValue(), 0.001)
		case labelValue(metric, "new_optional") == "later":
			hasLaterSchema = true
			assert.Equal(t, "first", labelValue(metric, "namespace"))
			assert.InDelta(t, 3.0, metric.GetGauge().GetValue(), 0.001)
		}
	}
	assert.True(t, hasOldSchema)
	assert.True(t, hasOmittedSchema)
	assert.True(t, hasLaterSchema)
}

func TestRecordDataPoint_NamespaceSeparation(t *testing.T) {
	collector, reg := isolatedCollector(t)
	record(t, collector, "namespace_metric", "operator", map[string]string{"namespace": "resource-one"}, 11)
	record(t, collector, "namespace_metric", "operator", map[string]string{"namespace": "resource-two"}, 22)

	samples := gatherGauge(t, reg, "namespace_metric")
	require.Len(t, samples, 2)
	values := make(map[string]float64, 2)
	for _, metric := range samples {
		values[labelValue(metric, "namespace")+"/"+labelValue(metric, "resource_namespace")] = metric.GetGauge().GetValue()
	}
	assert.InDelta(t, 11.0, values["operator/resource-one"], 0.001)
	assert.InDelta(t, 22.0, values["operator/resource-two"], 0.001)
}

func TestRecordDataPoint_DelimiterBearingLabelNames(t *testing.T) {
	collector, reg := isolatedCollector(t)
	record(t, collector, "delimiter_metric", "ns", map[string]string{"a,b": "comma", "c": "first"}, 1)
	record(t, collector, "delimiter_metric", "ns", map[string]string{"a": "single", "b,c": "second"}, 2)
	require.NoError(t, collector.recordDataPoint("delimiter_metric", "ns", map[string]string{"a,b": "comma", "c": "first"}, 3))

	samples := gatherGauge(t, reg, "delimiter_metric")
	require.Len(t, samples, 2)
	var first, second bool
	for _, metric := range samples {
		if labelValue(metric, "a,b") == "comma" && labelValue(metric, "c") == "first" {
			first = true
			assert.InDelta(t, 3.0, metric.GetGauge().GetValue(), 0.001)
		}
		if labelValue(metric, "a") == "single" && labelValue(metric, "b,c") == "second" {
			second = true
			assert.InDelta(t, 2.0, metric.GetGauge().GetValue(), 0.001)
		}
	}
	assert.True(t, first)
	assert.True(t, second)
}

func TestRecordDataPoint_NilDims(t *testing.T) {
	collector, reg := isolatedCollector(t)
	record(t, collector, "nil_dims_metric", "operator", nil, 7)
	samples := gatherGauge(t, reg, "nil_dims_metric")
	require.Len(t, samples, 1)
	assert.Equal(t, "operator", labelValue(samples[0], "namespace"))
	assert.InDelta(t, 7.0, samples[0].GetGauge().GetValue(), 0.001)
}

func TestRecordDataPoint_RejectsConflictingNamespaceLabels(t *testing.T) {
	collector, reg := isolatedCollector(t)
	err := collector.recordDataPoint("namespace_conflict_metric", "operator", map[string]string{
		"namespace":          "one",
		"resource_namespace": "two",
	}, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicts")
	assert.Empty(t, gatherGauge(t, reg, "namespace_conflict_metric"))
}

func TestRecordDataPoint_InvalidNameDoesNotPoisonGather(t *testing.T) {
	collector, reg := isolatedCollector(t)
	record(t, collector, "valid_metric_after_rejection", "ns", nil, 9)
	for _, name := range []string{"", string([]byte{0xff})} {
		err := collector.recordDataPoint(name, "ns", nil, 1)
		require.Error(t, err, "metric name %q", name)
	}
	samples := gatherGauge(t, reg, "valid_metric_after_rejection")
	require.Len(t, samples, 1)
	assert.InDelta(t, 9.0, samples[0].GetGauge().GetValue(), 0.001)
	_, err := reg.Gather()
	require.NoError(t, err)
}

func TestRecordDataPoint_InvalidProjectionLabelDoesNotPoisonGather(t *testing.T) {
	collector, reg := isolatedCollector(t)
	record(t, collector, "valid_label_metric", "ns", map[string]string{"valid": "value"}, 5)
	err := collector.recordDataPoint("invalid_label_metric", "ns", map[string]string{"__bad": "value"}, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid labels")
	assert.Empty(t, gatherGauge(t, reg, "invalid_label_metric"))
	samples := gatherGauge(t, reg, "valid_label_metric")
	require.Len(t, samples, 1)
	assert.InDelta(t, 5.0, samples[0].GetGauge().GetValue(), 0.001)
}

func TestRecordDataPoint_ConcurrentSchemasAndGather(t *testing.T) {
	collector, reg := isolatedCollector(t)
	const workers = 8
	const updates = 20
	errors := make(chan error, workers+1)
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Go(func() {
			dims := map[string]string{"worker": fmt.Sprint(worker)}
			if worker%2 == 0 {
				dims["optional"] = "present"
			}
			for value := range updates {
				if err := collector.recordDataPoint("concurrent_metric", "operator", dims, int64(value)); err != nil {
					errors <- err
					return
				}
			}
		})
	}
	wg.Go(func() {
		for range updates {
			if _, err := reg.Gather(); err != nil {
				errors <- err
				return
			}
		}
	})
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	samples := gatherGauge(t, reg, "concurrent_metric")
	require.Len(t, samples, workers)
	for _, sample := range samples {
		assert.Equal(t, float64(updates-1), sample.GetGauge().GetValue())
	}
}

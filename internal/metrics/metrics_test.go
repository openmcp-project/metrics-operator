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
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolatedRegistry replaces the package-level dynamicRegistry for the duration
// of a single test and restores it (along with dynamicGauges) in t.Cleanup.
func isolatedRegistry(t *testing.T) *prometheus.Registry {
	t.Helper()
	reg := prometheus.NewRegistry()

	dynamicMu.Lock()
	origReg := dynamicRegistry
	origGauges := dynamicGauges
	dynamicRegistry = reg
	dynamicGauges = map[string]*prometheus.GaugeVec{}
	dynamicMu.Unlock()

	t.Cleanup(func() {
		dynamicMu.Lock()
		dynamicRegistry = origReg
		dynamicGauges = origGauges
		dynamicMu.Unlock()
	})
	return reg
}

// gatherGauge returns all dto.Metric samples for the named metric family.
func gatherGauge(t *testing.T, reg prometheus.Gatherer, name string) []*dto.Metric {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() == name {
			return f.GetMetric()
		}
	}
	return nil
}

// labelValue returns the value of label `name` in m, or "" if absent.
func labelValue(m *dto.Metric, name string) string {
	for _, lp := range m.GetLabel() {
		if lp.GetName() == name {
			return lp.GetValue()
		}
	}
	return ""
}

func TestRecordDataPoint_ProjectionLabels(t *testing.T) {
	reg := isolatedRegistry(t)

	dims := map[string]string{
		"schedule_name":      "crate",
		"schedule_namespace": "velero",
		"schedule_phase":     "Enabled",
		"schedule_cron":      "0 0/8 * * *",
		"cluster":            "prod-cluster",
	}
	RecordDataPoint("velero_schedule_last_backup_seconds", "velero", dims, 1790755226)

	samples := gatherGauge(t, reg, "velero_schedule_last_backup_seconds")
	require.Len(t, samples, 1, "named gauge must emit exactly one sample")

	m := samples[0]
	assert.Equal(t, "crate", labelValue(m, "schedule_name"))
	assert.Equal(t, "velero", labelValue(m, "schedule_namespace"))
	assert.Equal(t, "Enabled", labelValue(m, "schedule_phase"))
	assert.Equal(t, "0 0/8 * * *", labelValue(m, "schedule_cron"))
	assert.Equal(t, "prod-cluster", labelValue(m, "cluster"))
	assert.Equal(t, "velero", labelValue(m, "namespace"))
	assert.InDelta(t, 1790755226.0, m.GetGauge().GetValue(), 0.001)
}

func TestRecordDataPoint_UpdateValue(t *testing.T) {
	reg := isolatedRegistry(t)

	dims := map[string]string{"schedule_name": "hourly"}

	RecordDataPoint("velero_backup_seconds", "ops", dims, 100)
	RecordDataPoint("velero_backup_seconds", "ops", dims, 200)

	samples := gatherGauge(t, reg, "velero_backup_seconds")
	require.Len(t, samples, 1)
	assert.InDelta(t, 200.0, samples[0].GetGauge().GetValue(), 0.001)
}

func TestRecordDataPoint_MultipleMetricNames(t *testing.T) {
	reg := isolatedRegistry(t)

	RecordDataPoint("metric_a", "ns", map[string]string{"x": "1"}, 10)
	RecordDataPoint("metric_b", "ns", map[string]string{"x": "2"}, 20)

	samplesA := gatherGauge(t, reg, "metric_a")
	samplesB := gatherGauge(t, reg, "metric_b")
	require.Len(t, samplesA, 1)
	require.Len(t, samplesB, 1)
	assert.InDelta(t, 10.0, samplesA[0].GetGauge().GetValue(), 0.001)
	assert.InDelta(t, 20.0, samplesB[0].GetGauge().GetValue(), 0.001)
}

func TestRecordDataPoint_NilDims_NoPanic(t *testing.T) {
	isolatedRegistry(t)
	assert.NotPanics(t, func() {
		RecordDataPoint("some_metric", "ns", nil, 0)
	})
}

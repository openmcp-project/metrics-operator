/*
Copyright 2026.

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
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func TestReservedMetricNamesRejectsDormantHistogramSuffix(t *testing.T) {
	registry := prometheus.NewRegistry()
	histogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "dormant_histogram",
		Help: "Histogram with no observations.",
	}, []string{"controller"})
	require.NoError(t, registry.Register(histogram))

	reserved := reservedMetricNames{}
	require.NoError(t, reserved.initialize(registry))
	for _, suffix := range []string{"_bucket", "_sum", "_count"} {
		err := reserved.validateInitialized("dormant_histogram" + suffix)
		require.Error(t, err)
		require.Contains(t, err.Error(), "dormant_histogram"+suffix)
	}
	families, err := registry.Gather()
	require.NoError(t, err)
	require.Empty(t, families, "initialization must not seed histogram observations")
}

func TestReservedMetricNamesRejectsExactCollision(t *testing.T) {
	registry := prometheus.NewRegistry()
	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "operator_metric",
		Help: "Operator metric.",
	}, []string{"controller"})
	require.NoError(t, registry.Register(gauge))

	reserved := reservedMetricNames{}
	require.NoError(t, reserved.initialize(registry))
	err := reserved.validateInitialized("operator_metric")
	require.Error(t, err)
	require.Contains(t, err.Error(), "operator_metric")
}

func TestReservedMetricNamesRejectsDormantGauge(t *testing.T) {
	reservedGaugeRegistry := prometheus.NewRegistry()
	reservedGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dormant_gauge",
		Help: "Gauge vector with no observations.",
	}, []string{"controller"})
	require.NoError(t, reservedGaugeRegistry.Register(reservedGauge))
	gaugeReserved := reservedMetricNames{}
	require.NoError(t, gaugeReserved.initialize(reservedGaugeRegistry))
	require.Error(t, gaugeReserved.validateInitialized("dormant_gauge"))
	families, err := reservedGaugeRegistry.Gather()
	require.NoError(t, err)
	require.Empty(t, families)
}

func TestReservedMetricNamesAcceptsBusinessMetricAndUTF8Name(t *testing.T) {
	reserved := reservedMetricNames{}
	require.NoError(t, reserved.initialize(prometheus.NewRegistry()))
	require.NoError(t, reserved.validateInitialized("business_metric_total"))
	// Prometheus' UTF8Validation scheme accepts valid non-ASCII names.
	require.NoError(t, reserved.validateInitialized("business.温度"))
}

func TestReservedMetricNamesRejectsInvalidUTF8(t *testing.T) {
	reserved := reservedMetricNames{}
	require.NoError(t, reserved.initialize(prometheus.NewRegistry()))
	err := reserved.validateInitialized(string([]byte{0xff}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid Prometheus metric name")
}

func TestReservedMetricNamesSnapshotOnlyOnce(t *testing.T) {
	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "operator_metric",
		Help: "Operator metric.",
	})))

	reserved := reservedMetricNames{}
	require.NoError(t, reserved.initialize(registry))
	require.NoError(t, registry.Register(prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "business_metric",
		Help: "Business metric recorded after snapshot.",
	})))

	require.NoError(t, reserved.initialize(registry))
	require.NoError(t, reserved.validateInitialized("business_metric"))
	err := reserved.validateInitialized("operator_metric")
	require.Error(t, err)
	require.Contains(t, err.Error(), "operator_metric")

}
func TestReservedMetricNamesSnapshotFailure(t *testing.T) {
	registry := failingGatherRegistry{
		desc: prometheus.NewDesc("operator_metric", "Operator metric.", nil, nil),
	}
	reserved := reservedMetricNames{}
	err := reserved.initialize(registry)
	require.Error(t, err)
	require.Contains(t, err.Error(), "snapshot failed")
}

type failingGatherRegistry struct {
	desc *prometheus.Desc
}

func (registry failingGatherRegistry) Describe(ch chan<- *prometheus.Desc) {
	ch <- registry.desc
}

func (failingGatherRegistry) Gather() ([]*dto.MetricFamily, error) {
	return nil, errors.New("snapshot failed")
}

func TestReservedMetricNamesRejectsLegacyEscapedCollision(t *testing.T) {
	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "reserved_metric",
		Help: "Operator metric.",
	})))

	reserved := reservedMetricNames{}
	require.NoError(t, reserved.initialize(registry))
	err := reserved.validateInitialized("reserved温metric")
	require.Error(t, err)
	require.Contains(t, err.Error(), "reserved_metric")
}

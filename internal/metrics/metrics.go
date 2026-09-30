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
	"sort"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// dynamicRegistry holds lazily-created GaugeVecs for named metrics.
// Key: "<metricName>/<sorted-comma-joined-labelNames>".
var (
	dynamicMu       sync.Mutex
	dynamicGauges   = map[string]*prometheus.GaugeVec{}
	dynamicRegistry prometheus.Registerer = ctrlmetrics.Registry
)

// namedGaugeKey returns the map key for a (metricName, labelNames) pair.
// labelNames must already be sorted.
func namedGaugeKey(metricName string, sortedLabels []string) string {
	return metricName + "/" + strings.Join(sortedLabels, ",")
}

// getOrCreateGauge returns an existing GaugeVec or registers a new one.
// labelNames must already be sorted.
func getOrCreateGauge(metricName string, sortedLabels []string) *prometheus.GaugeVec {
	key := namedGaugeKey(metricName, sortedLabels)

	dynamicMu.Lock()
	defer dynamicMu.Unlock()

	if g, ok := dynamicGauges[key]; ok {
		return g
	}

	g := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: metricName,
			Help: "Metric exposed by metrics-operator from spec.name with projection labels.",
		},
		sortedLabels,
	)

	// Register may fail if a metric with the same name but different labels was
	// registered before (e.g. label set changed between reconcile loops).  In that
	// case try to unregister the old one first, then re-register.
	if err := dynamicRegistry.Register(g); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			// The existing collector is compatible — reuse it.
			if existing, ok := are.ExistingCollector.(*prometheus.GaugeVec); ok {
				dynamicGauges[key] = existing
				return existing
			}
			// Incompatible existing collector — unregister and replace.
			dynamicRegistry.Unregister(are.ExistingCollector)
			if err2 := dynamicRegistry.Register(g); err2 != nil {
				// Give up; recording to this named gauge is skipped.
				return nil
			}
		} else {
			// Non-recoverable error (e.g. invalid metric name); skip named gauge.
			return nil
		}
	}

	dynamicGauges[key] = g
	return g
}

// RecordDataPoint registers a dynamically-named prometheus.GaugeVec for
// metricName (the CR spec.name) and records value with all dims plus namespace
// as individual Prometheus labels.
func RecordDataPoint(metricName, namespace string, dims map[string]string, value int64) {
	// Build the full label set: all dims plus "namespace".
	labels := make(prometheus.Labels, len(dims)+1)
	for k, v := range dims {
		labels[k] = v
	}
	labels["namespace"] = namespace

	// Collect and sort label names so that the GaugeVec key is stable.
	names := make([]string, 0, len(labels))
	for k := range labels {
		names = append(names, k)
	}
	sort.Strings(names)

	g := getOrCreateGauge(metricName, names)
	if g == nil {
		return
	}
	g.With(labels).Set(float64(value))
}

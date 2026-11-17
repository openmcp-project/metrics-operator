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
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/types"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const metricHelp = "Metric exposed by metrics-operator from spec.name with projection labels."

var dynamicMetrics = newDynamicGaugeCollector(ctrlmetrics.Registry)

type dynamicGaugeCollector struct {
	registerer prometheus.Registerer

	mu            sync.RWMutex
	registered    bool
	gauges        map[string]map[string]*prometheus.GaugeVec
	seriesByOwner map[metricOwner]ownerSnapshot
}

type metricOwner struct {
	uid types.UID
}

type ownerSnapshot struct {
	metricName string
	series     map[seriesKey]recordedSeries
}

type seriesKey struct {
	schema string
	labels string
}

type recordedSeries struct {
	schemaKey  string
	labelNames []string
	labels     prometheus.Labels
	value      int64
}

// MetricRecorder accumulates a metric CR's Prometheus series for one successful reconciliation.
type MetricRecorder struct {
	collector  *dynamicGaugeCollector
	metricName string
	namespace  string
	owner      metricOwner
	series     map[seriesKey]recordedSeries
}

func newDynamicGaugeCollector(registerer prometheus.Registerer) *dynamicGaugeCollector {
	return &dynamicGaugeCollector{
		registerer:    registerer,
		gauges:        make(map[string]map[string]*prometheus.GaugeVec),
		seriesByOwner: make(map[metricOwner]ownerSnapshot),
	}
}

func validateProjectionLabels(metricName string, labelNames []string) error {
	if err := prometheus.NewDesc(metricName, metricHelp, labelNames, nil).Err(); err != nil {
		return fmt.Errorf("invalid labels for metric %q: %w", metricName, err)
	}
	return nil
}

// Describe makes this collector unchecked because one metric family may have
// multiple projection label schemas.
func (*dynamicGaugeCollector) Describe(chan<- *prometheus.Desc) {}

func (c *dynamicGaugeCollector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, schemas := range c.gauges {
		for _, gauge := range schemas {
			gauge.Collect(ch)
		}
	}
}

func (c *dynamicGaugeCollector) newMetricRecorder(metricName, namespace string, uid types.UID) *MetricRecorder {
	return &MetricRecorder{
		collector:  c,
		metricName: metricName,
		namespace:  namespace,
		owner: metricOwner{
			uid: uid,
		},
		series: make(map[seriesKey]recordedSeries),
	}
}

// RecordDataPoint records a point in the reconciliation snapshot.
func (r *MetricRecorder) RecordDataPoint(dims map[string]string, value int64) error {
	series, err := r.collector.newRecordedSeries(r.metricName, r.namespace, dims, value)
	if err != nil {
		return err
	}
	r.series[seriesKey{schema: series.schemaKey, labels: labelValuesKey(series.labelNames, series.labels)}] = series
	return nil
}

// Commit replaces the owner's previous series with the reconciliation snapshot.
func (r *MetricRecorder) Commit() error {
	return r.collector.commit(r.owner, r.metricName, r.series)
}

func (c *dynamicGaugeCollector) newRecordedSeries(metricName, namespace string, dims map[string]string, value int64) (recordedSeries, error) {
	if err := ValidateMetricName(metricName); err != nil {
		return recordedSeries{}, err
	}

	resourceNamespace, hasNamespace := dims["namespace"]
	if existingResourceNamespace, ok := dims["resource_namespace"]; hasNamespace && ok && resourceNamespace != existingResourceNamespace {
		return recordedSeries{}, fmt.Errorf("projection labels namespace %q conflicts with resource_namespace %q", resourceNamespace, existingResourceNamespace)
	}

	labelNames := make([]string, 0, len(dims)+1)
	for name := range dims {
		if name == "namespace" {
			name = "resource_namespace"
		} else if name == "resource_namespace" && hasNamespace {
			continue
		}
		labelNames = append(labelNames, name)
	}
	labelNames = append(labelNames, "namespace")
	sort.Strings(labelNames)
	if err := validateProjectionLabels(metricName, labelNames); err != nil {
		return recordedSeries{}, err
	}

	labels := make(prometheus.Labels, len(labelNames))
	for name, value := range dims {
		if name == "namespace" {
			name = "resource_namespace"
		}
		labels[name] = value
	}
	labels["namespace"] = namespace
	return recordedSeries{
		schemaKey:  labelSchemaKey(labelNames),
		labelNames: labelNames,
		labels:     labels,
		value:      value,
	}, nil
}

func (c *dynamicGaugeCollector) commit(owner metricOwner, metricName string, current map[seriesKey]recordedSeries) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	schemas := c.gauges[metricName]
	for _, series := range current {
		gauge := schemas[series.schemaKey]
		if gauge == nil {
			gauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
				Name: metricName,
				Help: metricHelp,
			}, series.labelNames)
			if schemas == nil {
				schemas = make(map[string]*prometheus.GaugeVec)
				c.gauges[metricName] = schemas
			}
			schemas[series.schemaKey] = gauge
		}

		metric, err := gauge.GetMetricWith(series.labels)
		if err != nil {
			return fmt.Errorf("get metric %q with projection labels: %w", metricName, err)
		}
		metric.Set(float64(series.value))
	}

	if !c.registered && len(current) > 0 {
		if err := c.registerer.Register(c); err != nil {
			return fmt.Errorf("register dynamic metrics collector: %w", err)
		}
		c.registered = true
	}

	c.removeSeries(owner, c.seriesByOwner[owner], metricName, current)
	if len(current) == 0 {
		delete(c.seriesByOwner, owner)
		return nil
	}
	c.seriesByOwner[owner] = ownerSnapshot{metricName: metricName, series: current}
	return nil
}

func (c *dynamicGaugeCollector) remove(owner metricOwner) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeSeries(owner, c.seriesByOwner[owner], "", nil)
	delete(c.seriesByOwner, owner)
}

func (c *dynamicGaugeCollector) removeSeries(owner metricOwner, snapshot ownerSnapshot, currentMetricName string, current map[seriesKey]recordedSeries) {
	for key, series := range snapshot.series {
		if snapshot.metricName == currentMetricName {
			if _, exists := current[key]; exists {
				continue
			}
		}
		if c.isOwnedByAnotherMetric(owner, snapshot.metricName, key) {
			continue
		}
		c.gauges[snapshot.metricName][series.schemaKey].Delete(series.labels)
	}
}

func (c *dynamicGaugeCollector) isOwnedByAnotherMetric(owner metricOwner, metricName string, key seriesKey) bool {
	for otherOwner, snapshot := range c.seriesByOwner {
		if otherOwner != owner && snapshot.metricName == metricName {
			if _, exists := snapshot.series[key]; exists {
				return true
			}
		}
	}
	return false
}

func labelSchemaKey(sortedLabels []string) string {
	var key strings.Builder
	for _, label := range sortedLabels {
		key.WriteString(strconv.Itoa(len(label)))
		key.WriteByte(':')
		key.WriteString(label)
	}
	return key.String()
}

func labelValuesKey(labelNames []string, labels prometheus.Labels) string {
	var key strings.Builder
	for _, name := range labelNames {
		value := labels[name]
		key.WriteString(strconv.Itoa(len(value)))
		key.WriteByte(':')
		key.WriteString(value)
	}
	return key.String()
}

// NewMetricRecorder creates a reconciliation-scoped recorder for a metric CR.
func NewMetricRecorder(metricName, namespace string, uid types.UID) *MetricRecorder {
	return dynamicMetrics.newMetricRecorder(metricName, namespace, uid)
}

// RemoveMetricOwner removes every Prometheus series owned by a metric CR.
func RemoveMetricOwner(uid types.UID) {
	dynamicMetrics.remove(metricOwner{uid: uid})
}

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
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const metricHelp = "Metric exposed by metrics-operator from spec.name with projection labels."

var dynamicMetrics = newDynamicGaugeCollector(ctrlmetrics.Registry)

type dynamicGaugeCollector struct {
	registerer prometheus.Registerer

	mu         sync.RWMutex
	registered bool
	gauges     map[string]map[string]*prometheus.GaugeVec
}

func newDynamicGaugeCollector(registerer prometheus.Registerer) *dynamicGaugeCollector {
	return &dynamicGaugeCollector{
		registerer: registerer,
		gauges:     make(map[string]map[string]*prometheus.GaugeVec),
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

func (c *dynamicGaugeCollector) recordDataPoint(metricName, namespace string, dims map[string]string, value int64) error {
	if err := ValidateMetricName(metricName); err != nil {
		return err
	}

	resourceNamespace, hasNamespace := dims["namespace"]
	if existingResourceNamespace, ok := dims["resource_namespace"]; hasNamespace && ok && resourceNamespace != existingResourceNamespace {
		return fmt.Errorf("projection labels namespace %q conflicts with resource_namespace %q", resourceNamespace, existingResourceNamespace)
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
	schemaKey := labelSchemaKey(labelNames)
	labels := make(prometheus.Labels, len(labelNames))
	for name, value := range dims {
		if name == "namespace" {
			name = "resource_namespace"
		}
		labels[name] = value
	}
	labels["namespace"] = namespace

	c.mu.Lock()
	defer c.mu.Unlock()

	schemas := c.gauges[metricName]
	gauge := schemas[schemaKey]
	if gauge == nil {
		if err := validateProjectionLabels(metricName, labelNames); err != nil {
			return err
		}
		gauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: metricName,
			Help: metricHelp,
		}, labelNames)
	}

	metric, err := gauge.GetMetricWith(labels)
	if err != nil {
		return fmt.Errorf("get metric %q with projection labels: %w", metricName, err)
	}
	if !c.registered {
		if err := c.registerer.Register(c); err != nil {
			return fmt.Errorf("register dynamic metrics collector: %w", err)
		}
		c.registered = true
	}
	if schemas == nil {
		schemas = make(map[string]*prometheus.GaugeVec)
		c.gauges[metricName] = schemas
	}
	if schemas[schemaKey] == nil {
		schemas[schemaKey] = gauge
	}
	metric.Set(float64(value))
	return nil
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

// RecordDataPoint records a dynamically named gauge for metricName with
// projection labels and the CR namespace. The CR namespace is exposed as
// "namespace"; a projected namespace is exposed as "resource_namespace".
func RecordDataPoint(metricName, namespace string, dims map[string]string, value int64) error {
	return dynamicMetrics.recordDataPoint(metricName, namespace, dims, value)
}

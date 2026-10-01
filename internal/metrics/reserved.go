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
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

type reservedRegistry interface {
	Describe(chan<- *prometheus.Desc)
	Gather() ([]*dto.MetricFamily, error)
}

type reservedMetricNames struct {
	once  sync.Once
	names map[string]struct{}
	err   error
}

var operatorMetricNames reservedMetricNames

// InitializeReservedMetricNames snapshots operator metric names before business
// collectors are registered. Descriptors are included because vectors may not
// have emitted samples yet. Since a descriptor does not expose its metric type,
// histogram and summary suffixes are conservatively reserved for every name.
func InitializeReservedMetricNames() error {
	registry, ok := ctrlmetrics.Registry.(reservedRegistry)
	if !ok {
		return fmt.Errorf("controller-runtime metrics registry does not expose descriptors")
	}
	return operatorMetricNames.initialize(registry)
}

// ValidateMetricName validates the Prometheus UTF-8 metric-name syntax and
// rejects names which collide with an operator metric or its escaped forms.
func ValidateMetricName(metricName string) error {
	registry, ok := ctrlmetrics.Registry.(reservedRegistry)
	if !ok {
		return fmt.Errorf("initialize reserved Prometheus metric names: controller-runtime metrics registry does not expose descriptors")
	}
	if err := operatorMetricNames.initialize(registry); err != nil {
		return fmt.Errorf("initialize reserved Prometheus metric names: %w", err)
	}
	return operatorMetricNames.validateInitialized(metricName)
}
func (reserved *reservedMetricNames) initialize(registry reservedRegistry) error {
	reserved.once.Do(func() {
		reserved.names, reserved.err = snapshotReservedMetricNames(registry)
	})
	return reserved.err
}

func (reserved *reservedMetricNames) validateInitialized(metricName string) error {
	if !model.UTF8Validation.IsValidMetricName(metricName) {
		return fmt.Errorf("invalid Prometheus metric name %q: names must be non-empty valid UTF-8", metricName)
	}
	if _, exists := reserved.names[metricName]; exists {
		return fmt.Errorf("metric name %q collides with a reserved operator metric name %q", metricName, metricName)
	}
	if model.LegacyValidation.IsValidMetricName(metricName) {
		return nil
	}
	for _, scheme := range [...]model.EscapingScheme{model.UnderscoreEscaping, model.DotsEscaping, model.ValueEncodingEscaping} {
		candidate := model.EscapeName(metricName, scheme)
		if _, exists := reserved.names[candidate]; exists {
			return fmt.Errorf("metric name %q collides with a reserved operator metric name %q", metricName, candidate)
		}
	}
	return nil
}

func snapshotReservedMetricNames(registry reservedRegistry) (map[string]struct{}, error) {
	names := make(map[string]struct{})
	add := func(name string) {
		addReservedName(names, name)
	}

	descriptions := make(chan *prometheus.Desc)
	go func() {
		defer close(descriptions)
		registry.Describe(descriptions)
	}()
	var descriptorErr error
	for desc := range descriptions {
		if desc == nil {
			if descriptorErr == nil {
				descriptorErr = fmt.Errorf("operator metrics registry described a nil metric descriptor")
			}
			continue
		}
		if err := desc.Err(); err != nil {
			if descriptorErr == nil {
				descriptorErr = fmt.Errorf("operator metrics registry described invalid descriptor %s: %w", desc, err)
			}
			continue
		}
		var name string
		if _, err := fmt.Sscanf(desc.String(), "Desc{fqName: %q,", &name); err != nil {
			if descriptorErr == nil {
				descriptorErr = fmt.Errorf("read metric name from operator descriptor %s: %w", desc, err)
			}
			continue
		}
		add(name)
		// Desc exposes no metric type, so reserve possible histogram suffixes.
		for _, suffix := range [...]string{"_bucket", "_sum", "_count"} {
			add(name + suffix)
		}
	}

	families, err := registry.Gather()
	if err != nil {
		return nil, fmt.Errorf("gather operator metrics: %w", err)
	}
	if descriptorErr != nil {
		return nil, descriptorErr
	}
	for _, family := range families {
		name := family.GetName()
		add(name)
		if family.GetType() == dto.MetricType_HISTOGRAM || family.GetType() == dto.MetricType_SUMMARY {
			for _, suffix := range []string{"_bucket", "_sum", "_count"} {
				add(name + suffix)
			}
		}
	}

	return names, nil
}

func addReservedName(names map[string]struct{}, name string) {
	names[name] = struct{}{}
	if model.LegacyValidation.IsValidMetricName(name) {
		return
	}
	for _, scheme := range [...]model.EscapingScheme{model.UnderscoreEscaping, model.DotsEscaping, model.ValueEncodingEscaping} {
		names[model.EscapeName(name, scheme)] = struct{}{}
	}
}

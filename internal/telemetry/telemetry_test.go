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

package telemetry

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.uber.org/zap/zapcore"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestResourceIncludesTelemetrySDKAttributes(t *testing.T) {
	attrs := Resource().Set()
	for _, key := range []attribute.Key{
		"service.name",
		"telemetry.sdk.language",
		"telemetry.sdk.name",
		"telemetry.sdk.version",
	} {
		if value, ok := attrs.Value(key); !ok || value.AsString() == "" {
			t.Errorf("resource attribute %q is missing or empty", key)
		}
	}
}

func TestRegisterSDKInfoMetric(t *testing.T) {
	reg := prometheus.NewRegistry()
	for range 2 {
		if err := RegisterSDKInfoMetric(reg); err != nil {
			t.Fatalf("RegisterSDKInfoMetric() error = %v", err)
		}
	}

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if len(families) != 1 || families[0].GetName() != "target_info" || len(families[0].Metric) != 1 {
		t.Fatalf("unexpected metric families: %v", families)
	}

	labels := map[string]string{}
	for _, label := range families[0].Metric[0].Label {
		labels[label.GetName()] = label.GetValue()
	}
	want := map[string]string{
		"telemetry_sdk_name":     "opentelemetry",
		"telemetry_sdk_language": "go",
		"telemetry_sdk_version":  sdk.Version(),
	}
	for key, value := range want {
		if labels[key] != value {
			t.Errorf("%s = %q, want %q", key, labels[key], value)
		}
	}
}

func TestRegisterSDKInfoMetricRejectsConflict(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "target_info",
		Help: "Different target metadata.",
	}))
	if err := RegisterSDKInfoMetric(reg); err == nil {
		t.Fatal("RegisterSDKInfoMetric() should reject a conflicting descriptor")
	}
}

func TestLogsExportConfigured(t *testing.T) {
	t.Run("disabled without endpoint", func(t *testing.T) {
		t.Setenv("OTEL_LOGS_EXPORTER", "")
		t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", "")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
		if logsExportConfigured() {
			t.Fatal("log export should be disabled")
		}
	})

	t.Run("enabled by common endpoint", func(t *testing.T) {
		t.Setenv("OTEL_LOGS_EXPORTER", "")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4317")
		if !logsExportConfigured() {
			t.Fatal("log export should be enabled")
		}
	})

	t.Run("none overrides endpoint", func(t *testing.T) {
		t.Setenv("OTEL_LOGS_EXPORTER", "none")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector:4317")
		if logsExportConfigured() {
			t.Fatal("log export should be disabled")
		}
	})
}

func TestNewLoggerHonorsConfiguredLevel(t *testing.T) {
	exporter := &recordingLogExporter{}
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)))
	previous := otel.GetLoggerProvider()
	otel.SetLoggerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetLoggerProvider(previous)
	})

	logger := NewLogger(&crzap.Options{Level: zapcore.Level(-1)})
	if !logger.V(1).Enabled() {
		t.Error("V(1) should be enabled")
	}
	if logger.V(8).Enabled() {
		t.Error("V(8) should be disabled")
	}
}

func TestNewLoggerExportsTelemetryResource(t *testing.T) {
	exporter := &recordingLogExporter{}
	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(Resource()),
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter)),
	)
	previous := otel.GetLoggerProvider()
	otel.SetLoggerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetLoggerProvider(previous)
	})

	NewLogger(&crzap.Options{}).Info("test record")
	if len(exporter.records) != 1 {
		t.Fatalf("exported records = %d, want 1", len(exporter.records))
	}
	attrs := exporter.records[0].Resource().Set()
	for _, key := range []attribute.Key{
		"service.name",
		"telemetry.sdk.language",
		"telemetry.sdk.name",
		"telemetry.sdk.version",
	} {
		if value, ok := attrs.Value(key); !ok || value.AsString() == "" {
			t.Errorf("log resource attribute %q is missing or empty", key)
		}
	}
}

type recordingLogExporter struct {
	records []sdklog.Record
}

func (e *recordingLogExporter) Export(_ context.Context, records []sdklog.Record) error {
	for i := range records {
		e.records = append(e.records, records[i].Clone())
	}
	return nil
}

func (*recordingLogExporter) Shutdown(context.Context) error   { return nil }
func (*recordingLogExporter) ForceFlush(context.Context) error { return nil }

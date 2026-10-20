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
	"errors"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel"
	otlploggrpc "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/sdk"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	uberzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const instrumentationScope = "github.com/openmcp-project/metrics-operator"

// ShutdownFunc flushes and shuts down the log provider.
type ShutdownFunc func(context.Context) error

var noopShutdown ShutdownFunc = func(context.Context) error { return nil }

// Resource returns the SDK's process-wide default resource.
func Resource() *resource.Resource {
	return resource.Default()
}

// RegisterSDKInfoMetric exposes OTel SDK identity to Prometheus receivers via
// the standard target_info series.
func RegisterSDKInfoMetric(reg prometheus.Registerer) error {
	info := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "target_info",
		Help: "Telemetry SDK metadata for this scrape target.",
		ConstLabels: prometheus.Labels{
			"telemetry_sdk_name":     "opentelemetry",
			"telemetry_sdk_language": "go",
			"telemetry_sdk_version":  sdk.Version(),
		},
	})
	info.Set(1)
	var alreadyRegistered prometheus.AlreadyRegisteredError
	if err := reg.Register(info); err != nil && !errors.As(err, &alreadyRegistered) {
		return err
	}
	return nil
}

// SetupLogs enables OTLP/gRPC log export when standard OTel environment
// variables configure an exporter endpoint.
func SetupLogs(ctx context.Context) (ShutdownFunc, error) {
	if !logsExportConfigured() {
		return noopShutdown, nil
	}

	exporter, err := otlploggrpc.New(ctx)
	if err != nil {
		return noopShutdown, err
	}

	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(Resource()),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	)
	otel.SetLoggerProvider(provider)

	return provider.Shutdown, nil
}

func logsExportConfigured() bool {
	exporter := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_LOGS_EXPORTER")))
	if exporter == "none" {
		return false
	}
	return exporter == "otlp" || os.Getenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != ""
}

// NewLogger tees the controller-runtime zap logger to the global OTel logger
// provider while retaining stdout output.
func NewLogger(opts *crzap.Options) logr.Logger {
	raw := crzap.NewRaw(crzap.UseFlagOptions(opts))
	var otelCore zapcore.Core = otelzap.NewCore(instrumentationScope)
	if opts.Level != nil {
		if gated, err := zapcore.NewIncreaseLevelCore(otelCore, opts.Level); err == nil {
			otelCore = gated
		}
	}
	teed := raw.WithOptions(uberzap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(core, otelCore)
	}))
	return zapr.NewLogger(teed)
}

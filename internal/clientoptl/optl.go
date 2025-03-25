package clientoptl

import (
	"context"
	"fmt"
	"path"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// MetricClient represents a metric client
type MetricClient struct {
	meter           metric.Meter
	manualReader    *sdkmetric.ManualReader
	metricsExporter *otlpmetrichttp.Exporter
}

// Metric represents a metric
type Metric struct {
	// default to gauge for now, as count requires the client to keep track of values (total)
	// we just want to send the current value/state always, hence gauge metric
	gauge metric.Int64Gauge
}

// DataPoint represents a single data point
type DataPoint struct {
	Dimensions map[string]string
	Value      int64
}

// NewDataPoint creates a new data point
func NewDataPoint() *DataPoint {
	return &DataPoint{
		Dimensions: make(map[string]string),
	}
}

// AddDimension adds a dimension to the data point
func (dp *DataPoint) AddDimension(key, value string) *DataPoint {
	dp.Dimensions[key] = value
	return dp
}

// SetValue sets the value of the data point
func (dp *DataPoint) SetValue(value int64) *DataPoint {
	dp.Value = value
	return dp
}

// NewMetricClient creates a new metric client
func NewMetricClient(ctx context.Context, dtAPIHost, dtAPIBasePath, dtAPIToken string) (*MetricClient, error) {
	authHeader := map[string]string{"Authorization": "Api-Token " + dtAPIToken}

	deltaTemporalitySelector := func(sdkmetric.InstrumentKind) metricdata.Temporality {
		return metricdata.DeltaTemporality
	}

	urlPath := path.Join(dtAPIBasePath, "/otlp/v1/metrics")

	metricsExporter, err := otlpmetrichttp.New(
		ctx,
		otlpmetrichttp.WithEndpoint(dtAPIHost),
		otlpmetrichttp.WithURLPath(urlPath),
		otlpmetrichttp.WithHeaders(authHeader),
		otlpmetrichttp.WithTemporalitySelector(deltaTemporalitySelector),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// manual reader allows us to collect metrics and send them manually
	// IF and ONLY IF necessary, we can force shutdown to flush any pending metrics
	manualReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(manualReader),
	)

	otel.SetMeterProvider(mp)

	return &MetricClient{
		manualReader:    manualReader,
		metricsExporter: metricsExporter,
	}, nil
}

// SetMeter creates a new meter with the given name
// A Meter is an interface for creating instruments (like counters, gauges, and histograms) that are used to record measurements.
// Used to group related metrics together.
func (mc *MetricClient) SetMeter(name string) {
	mc.meter = otel.Meter(name)
}

// NewMetric creates a new metric with the given name
func (mc *MetricClient) NewMetric(name string) (*Metric, error) {
	gauge, err := mc.meter.Int64Gauge(name)

	if err != nil {
		return nil, fmt.Errorf("failed to create gauge metric: %w", err)
	}

	return &Metric{
		gauge: gauge,
	}, nil
}

// RecordMetrics records the given series of data points
func (mc *Metric) RecordMetrics(ctx context.Context, series ...*DataPoint) error {

	for _, s := range series {
		var attrs []attribute.KeyValue
		for k, v := range s.Dimensions {
			attrs = append(attrs, attribute.String(k, v))
		}

		mc.gauge.Record(ctx, s.Value, metric.WithAttributes(attrs...))
	}

	return nil
}

// ExportMetrics sends the collected metrics to the exporter
func (mc *MetricClient) ExportMetrics(ctx context.Context) error {
	resourceMetrics := metricdata.ResourceMetrics{}
	err := mc.manualReader.Collect(ctx, &resourceMetrics)
	if err != nil {
		return fmt.Errorf("failed to collect metrics: %w", err)
	}

	err = mc.metricsExporter.Export(ctx, &resourceMetrics)
	if err != nil {
		return fmt.Errorf("failed to export metrics: %w", err)
	}

	return nil
}

// Close shuts down the metric client
func (mc *MetricClient) Close(ctx context.Context) error {
	return mc.metricsExporter.Shutdown(ctx)
}

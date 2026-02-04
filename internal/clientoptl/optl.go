package clientoptl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/openmcp-project/metrics-operator/internal/common"
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
func NewMetricClient(ctx context.Context, credentials *common.DataSinkCredentials) (*MetricClient, error) {

	deltaTemporalitySelector := func(sdkmetric.InstrumentKind) metricdata.Temporality {
		return metricdata.DeltaTemporality
	}

	// Parse the dtAPIHost URL to extract host and path components
	// dtAPIHost is the full endpoint from DataSink, e.g., "https://.../otlp/v1/metrics"
	parsedURL, err := url.Parse(credentials.Host)
	if err != nil {
		return nil, fmt.Errorf("failed to parse endpoint URL: %w", err)
	}

	// Construct OTLP options with proper URL parsing
	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(parsedURL.Host),
		otlpmetrichttp.WithURLPath(parsedURL.Path), // Use the path directly from the DataSink endpoint
		otlpmetrichttp.WithTemporalitySelector(deltaTemporalitySelector),
	}

	if credentials.APIKey != nil {
		authHeader := map[string]string{"Authorization": "Api-Token " + credentials.APIKey.Token}
		opts = append(opts, otlpmetrichttp.WithHeaders(authHeader))
	}

	if credentials.Certificate != nil {
		tlsConfig, err := createTLSConfig(
			credentials.Certificate.ClientCert,
			credentials.Certificate.ClientKey,
			credentials.Certificate.CACert,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS config: %w", err)
		}
		opts = append(opts, otlpmetrichttp.WithTLSClientConfig(tlsConfig))
	}

	// Add insecure option if scheme is http
	if parsedURL.Scheme == "http" {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	metricsExporter, err := otlpmetrichttp.New(ctx, opts...)
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

func createTLSConfig(clientCert, clientKey, caCert []byte) (*tls.Config, error) {
	// Load client certificate and key
	cert, err := tls.X509KeyPair(clientCert, clientKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	// Add CA certificate if provided
	if len(caCert) > 0 {
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to append CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}

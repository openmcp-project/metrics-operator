package clientoptl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	grpccredentials "google.golang.org/grpc/credentials"

	"github.com/openmcp-project/metrics-operator/internal/common"
)

const (
	protocolOTLPHTTPInsecure = "http"
	protocolOTLPHTTPSecure   = "https"
	protocolOTLPGRPCInsecure = "grpc"
	protocolOTLPGRPCSecure   = "grpcs"
)

// MetricClient represents a metric client
type MetricClient struct {
	meter           metric.Meter
	manualReader    *sdkmetric.ManualReader
	metricsExporter MetricsExporter
}

// MetricsExporter is the common interface for metric exporters
type MetricsExporter interface {
	Export(ctx context.Context, rm *metricdata.ResourceMetrics) error
	Shutdown(ctx context.Context) error
}

func isHTTPProtocol(scheme string) bool {
	return scheme == protocolOTLPHTTPInsecure || scheme == protocolOTLPHTTPSecure
}

func isGRPCProtocol(scheme string) bool {
	return scheme == protocolOTLPGRPCInsecure || scheme == protocolOTLPGRPCSecure
}

func isSecureProtocol(scheme string) bool {
	return scheme == protocolOTLPHTTPSecure || scheme == protocolOTLPGRPCSecure
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

	var metricsExporter MetricsExporter

	if isHTTPProtocol(parsedURL.Scheme) {
		metricsExporter, err = newMetricsClientHttp(ctx, credentials, parsedURL, deltaTemporalitySelector)
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP metrics client: %w", err)
		}
	} else if isGRPCProtocol(parsedURL.Scheme) {
		metricsExporter, err = newMetricsClientGrpc(ctx, credentials, parsedURL, deltaTemporalitySelector)
		if err != nil {
			return nil, fmt.Errorf("failed to create gRPC metrics client: %w", err)
		}
	} else {
		return nil, fmt.Errorf("unsupported protocol scheme, got %s, want http|https|grpc|grpcs", parsedURL.Scheme)
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

// newMetricsClientHttp creates a new OTLP HTTP metrics exporter
func newMetricsClientHttp(ctx context.Context, credentials *common.DataSinkCredentials, parsedURL *url.URL, temporalitySelector sdkmetric.TemporalitySelector) (*otlpmetrichttp.Exporter, error) {
	// Construct OTLP options with proper URL parsing
	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(parsedURL.Host),
		otlpmetrichttp.WithURLPath(parsedURL.Path), // Use the path directly from the DataSink endpoint
		otlpmetrichttp.WithTemporalitySelector(temporalitySelector),
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
	if !isSecureProtocol(parsedURL.Scheme) {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	metricsExporter, err := otlpmetrichttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	return metricsExporter, nil
}

// newMetricsClientGrpc creates a new OTLP gRPC metrics exporter
func newMetricsClientGrpc(ctx context.Context, credentials *common.DataSinkCredentials, parsedURL *url.URL, temporalitySelector sdkmetric.TemporalitySelector) (*otlpmetricgrpc.Exporter, error) {
	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(parsedURL.Host),
		otlpmetricgrpc.WithTemporalitySelector(temporalitySelector),
	}

	if credentials.APIKey != nil {
		authHeader := map[string]string{"Authorization": "Api-Token " + credentials.APIKey.Token}
		opts = append(opts, otlpmetricgrpc.WithHeaders(authHeader))
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
		tlsCredentials := grpccredentials.NewTLS(tlsConfig)
		opts = append(opts, otlpmetricgrpc.WithTLSCredentials(tlsCredentials))
	}

	if !isSecureProtocol(parsedURL.Scheme) {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}

	metricsExporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP gRPC exporter: %w", err)
	}

	return metricsExporter, nil
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

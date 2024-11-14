package clientlite

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// MetricClient represents a client for sending metrics to Dynatrace
type MetricClient struct {
	dynatraceURL string
	apiToken     string
	httpClient   *http.Client
}

const (
	METRICS_ENDPOINT = "/metrics/ingest"
)

// NewMetricClient creates a new MetricClient
func NewMetricClient(baseURL, endpoint, apiToken string) *MetricClient {
	// Create the base URL
	dynatraceBaseURL := &url.URL{
		Scheme: "https",
		Host:   baseURL,
	}

	fullPath := path.Join(endpoint, METRICS_ENDPOINT)

	// Combine the base URL with the endpoint
	fullURL := dynatraceBaseURL.ResolveReference(&url.URL{Path: fullPath})

	return &MetricClient{
		dynatraceURL: fullURL.String(),
		apiToken:     apiToken,
		httpClient:   &http.Client{},
	}
}

// Metric represents a single metric to be sent to Dynatrace
type Metric struct {
	Name       string
	dimensions map[string]string
	gaugeValue float64
}

// NewMetric creates a new Metric with the given name
func NewMetric(name string) *Metric {
	return &Metric{
		Name:       name,
		dimensions: make(map[string]string),
	}
}

// AddDimension adds a dimension to the metric
func (m *Metric) AddDimension(key, value string) *Metric {

	if value == "" {
		// dont add empty values
		return m
	}

	m.dimensions[key] = value
	return m
}

// SetGaugeValue sets the gauge value for the metric
func (m *Metric) SetGaugeValue(value float64) *Metric {
	m.gaugeValue = value
	return m
}

// formatMetric formats a single metric into the Dynatrace line protocol
func (m *Metric) format() string {
	// the general format of the payload is
	// format,dataPoint timestamp
	// The format of the timestamp is UTC milliseconds. The allowed range is between 1 hour into the past and 10 minutes into the future from now. Data points with timestamps outside of this range are rejected.
	//
	//If no timestamp is provided, the current timestamp of the server is used.

	var dimPairs []string
	for k, v := range m.dimensions {
		dimPairs = append(dimPairs, fmt.Sprintf("%s=\"%s\"", k, v)) // Note: need to add quotes for multi-word values, otherwise an exception is thrown
	}
	dimensions := strings.Join(dimPairs, ",")

	if dimensions != "" {
		dimensions = "," + dimensions
	}

	return fmt.Sprintf("%s%s gauge,%.2f", m.Name, dimensions, m.gaugeValue)
}

// SendMetrics sends multiple metrics to Dynatrace
func (c *MetricClient) SendMetrics(metrics ...*Metric) error {
	var metricLines []string

	for _, metric := range metrics {
		metricLines = append(metricLines, metric.format())
	}

	payload := strings.Join(metricLines, "\n")

	req, err := http.NewRequest("POST", c.dynatraceURL, bytes.NewBufferString(payload))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Authorization", "Api-Token "+c.apiToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

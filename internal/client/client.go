package client

import (
	"context"
	"net/http"
	"net/url"

	dyn "github.com/dynatrace-ace/dynatrace-go-api-client/api/v2/environment/dynatrace"
)

// DynatraceClient is a wrapper for the dynatrace-go-client, this interface only implements functions regarding metrics.
type DynatraceClient struct {
	Client        *dyn.APIClient
	configuration *dyn.Configuration
}

// NewClient Is used to create a new DynatraceClient,
//
// Path: Is the path after the domain to the api https://example.com/some/path/api/v2 (path: /some/path/api/v2)
//
// Token: An access token with read, write, create and update rights for Metrics and SettingObjects
//
// Host: Is the domain without scheme and path: https://example.com/some/path (domain: example.com)
func NewClient(host string, apiPath string, token string) DynatraceClient {
	d := DynatraceClient{
		configuration: &dyn.Configuration{
			Host: url.PathEscape(host),
			Servers: dyn.ServerConfigurations{
				{
					URL: apiPath,
				},
			},
			Scheme:        "https",
			DefaultHeader: map[string]string{"Authorization": "Api-Token " + token},
		},
		Client: dyn.NewAPIClient(
			&dyn.Configuration{
				Host: url.PathEscape(host),
				Servers: dyn.ServerConfigurations{
					{
						URL: apiPath,
					},
				},
				Scheme:        "https",
				DefaultHeader: map[string]string{"Authorization": "Api-Token " + token},
			},
		),
	}

	return d
}

// SendMetric Sends a metric, and if the metric isn't created this will also create the metric
// This will not include the generateion or POST request of the metadata
// Use this to send single datapoints of a metric to the Dynatrace Backend
func (d *DynatraceClient) SendMetric(ctx context.Context, metric MetricMetadata) (*http.Response, error) {
	body := metric.GenerateMetricBody()
	res, err := d.Client.MetricsApi.Ingest(ctx).Body(body).Execute()

	return res, err
}

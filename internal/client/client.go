package client

import (
	"context"
	"net/http"
	"net/url"

	dyn "github.com/dynatrace-ace/dynatrace-go-api-client/api/v2/environment/dynatrace"
)

// AbstractDynatraceClient This interface is a wrapper for the dynatrace-go-client, this interface only implements functions regarding metrics.
// This wrapper also includes the ability to add metadata to your metric which uses the SettingsObjectAPI in the background.
type AbstractDynatraceClient interface {
	NewClient(url *string, token string) *dyn.APIClient

	// Metrics: Speak directly to MetricsAPI
	SendMetric(ctx context.Context, metric MetricMetadata) (*http.Response, error)
	GetMetric(ctx context.Context, id string) (dyn.MetricDescriptor, *http.Response, error)
	GetAllMetrics(ctx context.Context) (dyn.MetricDescriptorCollection, *http.Response, error)
	DeleteMetric(ctx context.Context, id string) (*http.Response, error)

	// Metrics Metadata: Speaks to SettingsObjectAPI
	SendMetricMetadata(ctx context.Context, metric MetricMetadata) ([]dyn.SettingsObjectResponse, *http.Response, error)
	GetMetricMetadata(ctx context.Context, id string) (dyn.ObjectsList, *http.Response, error)
	DeleteMetricMetadata(ctx context.Context, objectID string) (*http.Response, error)
	UpdateMetricMetadata(ctx context.Context) (dyn.SettingsObjectResponse, *http.Response, error)
}

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

// GetAllMetrics returns all available metrics that Dynatrace has to over, this is not limited to anything.
func (d *DynatraceClient) GetAllMetrics(ctx context.Context) (dyn.MetricDescriptorCollection, *http.Response, error) {
	coll, res, err := d.Client.MetricsApi.AllMetrics(ctx).Execute()
	return coll, res, err
}

// GetMetric get a specific metric
// id: id of the metric you want (not the display name)
func (d *DynatraceClient) GetMetric(ctx context.Context, id string) (dyn.MetricDescriptor, *http.Response, error) {
	des, res, err := d.Client.MetricsApi.Metric(ctx, id).Execute()
	return des, res, err
}

// DeleteMetric deletes a metric
// id: id of the metric you want (not the display name)
func (d *DynatraceClient) DeleteMetric(ctx context.Context, id string) (*http.Response, error) {
	res, err := d.Client.MetricsApi.Delete(ctx, id).Execute()
	return res, err
}

// SendMetricMetadata sends the metadata of the metric
// this is a big request, this could cause overhead if send with every single datapoint
func (d *DynatraceClient) SendMetricMetadata(ctx context.Context, metric MetricMetadata) ([]dyn.SettingsObjectResponse, *http.Response, error) {
	settings, err := metric.GenerateSettingsObjects()
	if err != nil {
		return []dyn.SettingsObjectResponse{}, &http.Response{}, err
	}
	collPost, resPost, errPost := d.Client.SettingsObjectsApi.PostSettingsObjects(ctx).SettingsObjectCreate(settings).Execute()
	return collPost, resPost, errPost
}

// GetMetricMetadata gets the metadata of a specific metric
// id: id of the metric you want (not the display name)
func (d *DynatraceClient) GetMetricMetadata(ctx context.Context, id string) (dyn.ObjectsList, *http.Response, error) {
	coll, res, err := d.Client.SettingsObjectsApi.GetSettingsObjects(ctx).SchemaIds("builtin:metric.metadata").Scopes("metric-" + id).Execute()
	return coll, res, err
}

// DeleteMetricMetadata deletes the metadata of a specific metric
// objectId: this id can be optained by making a get request (is not the normal id of an metric)
func (d *DynatraceClient) DeleteMetricMetadata(ctx context.Context, objectID string) (*http.Response, error) {
	resDel, errDel := d.Client.SettingsObjectsApi.DeleteSettingsObjectByObjectId(ctx, objectID).Execute()
	return resDel, errDel
}

// UpdateMetricMetadata updates the metadata of a specific metric
// objectId: this id can be optained by making a get request (is not the normal id of an metric)
// metric: the updated complete metricMetadata object
// updateToken: can again be obtained by making a get request for the metric you want to update
func (d *DynatraceClient) UpdateMetricMetadata(ctx context.Context, objectID string, metric MetricMetadata, updateToken string) (dyn.SettingsObjectResponse, *http.Response, error) {
	settings, err := metric.GenerateUpdateSettings(objectID, metric, updateToken)
	if err != nil {
		return dyn.SettingsObjectResponse{}, &http.Response{}, err
	}
	colUpd, resUpd, errUpd := d.Client.SettingsObjectsApi.PutSettingsObjectByObjectId(ctx, objectID).SettingsObjectUpdate(settings).Execute()
	return colUpd, resUpd, errUpd
}

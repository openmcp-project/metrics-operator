package client

import (
	"context"
	"net/http"
	"net/url"

	dyn "github.com/dynatrace-ace/dynatrace-go-api-client/api/v2/environment/dynatrace"
)

// This interface is a wrapper for the dynatrace-go-client, this interface only implements functions regarding metrics.
// This wrapper also includes the ability to add metadata to your metric which uses the SettingsObjectAPI in the background.
type AbstractDynatraceClient interface {
	NewClient(url *string, token string) *dyn.APIClient

	// Metrics: Speak directly to MetricsAPI
	SendMetric(metric MetricMetadata) (*http.Response, error)
	GetMetric(id string) (dyn.MetricDescriptor, *http.Response, error)
	GetAllMetrics() (dyn.MetricDescriptorCollection, *http.Response, error)
	DeleteMetric(id string) (*http.Response, error)

	// Metrics Metadata: Speaks to SettingsObjectAPI
	SendMetricMetadata(metric MetricMetadata) ([]dyn.SettingsObjectResponse, *http.Response, error)
	GetMetricMetadata(id string) (dyn.ObjectsList, *http.Response, error)
	DeleteMetricMetadata(objectId string) (*http.Response, error)
	UpdateMetricMetadata() (dyn.SettingsObjectResponse, *http.Response, error)
}

type DynatraceClient struct {
	Client        *dyn.APIClient
	configuration *dyn.Configuration
}

// Is used to create a new DynatraceClient,
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

// Sends a metric, and if the metric isn't created this will also create the metric
// This will not include the generateion or POST request of the metadata
// Use this to send single datapoints of a metric to the Dynatrace Backend
func (d *DynatraceClient) SendMetric(metric MetricMetadata) (*http.Response, error) {
	body := metric.GenerateMetricBody()
	res, err := d.Client.MetricsApi.Ingest(context.Background()).Body(body).Execute()

	return res, err
}

// returns all available metrics that Dynatrace has to over, this is not limited to anything.
func (d *DynatraceClient) GetAllMetrics() (dyn.MetricDescriptorCollection, *http.Response, error) {
	coll, res, err := d.Client.MetricsApi.AllMetrics(context.Background()).Execute()
	return coll, res, err
}

// get a specific metric
// id: id of the metric you want (not the display name)
func (d *DynatraceClient) GetMetric(id string) (dyn.MetricDescriptor, *http.Response, error) {
	des, res, err := d.Client.MetricsApi.Metric(context.Background(), id).Execute()
	return des, res, err
}

// deletes a metric
// id: id of the metric you want (not the display name)
func (d *DynatraceClient) DeleteMetric(id string) (*http.Response, error) {
	res, err := d.Client.MetricsApi.Delete(context.Background(), id).Execute()
	return res, err
}

// sends the metadata of the metric
// this is a big request, this could cause overhead if send with every single datapoint
func (d *DynatraceClient) SendMetricMetadata(metric MetricMetadata) ([]dyn.SettingsObjectResponse, *http.Response, error) {
	settings, err := metric.GenerateSettingsObjects()
	if err != nil {
		return []dyn.SettingsObjectResponse{}, &http.Response{}, err
	}
	collPost, resPost, errPost := d.Client.SettingsObjectsApi.PostSettingsObjects(context.Background()).SettingsObjectCreate(settings).Execute()
	return collPost, resPost, errPost
}

// gets the metadata of a specific metric
// id: id of the metric you want (not the display name)
func (d *DynatraceClient) GetMetricMetadata(id string) (dyn.ObjectsList, *http.Response, error) {
	coll, res, err := d.Client.SettingsObjectsApi.GetSettingsObjects(context.Background()).SchemaIds("builtin:metric.metadata").Scopes("metric-" + id).Execute()
	return coll, res, err
}

// deletes the metadata of a specific metric
// objectId: this id can be optained by making a get request (is not the normal id of an metric)
func (d *DynatraceClient) DeleteMetricMetadata(objectId string) (*http.Response, error) {
	resDel, errDel := d.Client.SettingsObjectsApi.DeleteSettingsObjectByObjectId(context.Background(), objectId).Execute()
	return resDel, errDel
}

// updates the metadata of a specific metric
// objectId: this id can be optained by making a get request (is not the normal id of an metric)
// metric: the updated complete metricMetadata object
// updateToken: can again be obtained by making a get request for the metric you want to update
func (d *DynatraceClient) UpdateMetricMetadata(objectId string, metric MetricMetadata, updateToken string) (dyn.SettingsObjectResponse, *http.Response, error) {
	settings, err := metric.GenerateUpdateSettings(objectId, metric, updateToken)
	if err != nil {
		return dyn.SettingsObjectResponse{}, &http.Response{}, err
	}
	colUpd, resUpd, errUpd := d.Client.SettingsObjectsApi.PutSettingsObjectByObjectId(context.Background(), objectId).SettingsObjectUpdate(settings).Execute()
	return colUpd, resUpd, errUpd
}

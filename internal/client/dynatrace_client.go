package dynclient

import (
	"context"
	"fmt"
	"os"

	openapiclient "github.com/dynatrace-ace/dynatrace-go-api-client/api/v2/environment/dynatrace"
)

func NewDynatraceClientPoc(url string, apiToken string) *DynatraceClientPoc {
	return &DynatraceClientPoc{
		client:   apiClient(url),
		apiToken: apiToken,
	}
}

type DynatraceClientPoc struct {
	client   *openapiclient.APIClient
	apiToken string
}

func (dynClient *DynatraceClientPoc) PostMetric(ctx context.Context, kind, group, version string, value int) {
	metric_format := "stephan.managedresources.count,kind=%s,group=%s,version=%s %d"
	body := fmt.Sprintf(metric_format, kind, group, version, value)

	resp, err := dynClient.client.MetricsApi.Ingest(dynClient.authorizedCtx(ctx)).Body(body).Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when calling `MetricsApi.Ingest``: %v\n", err)
	}
	// response from `AllMetrics`: MetricDescriptorCollection
	fmt.Fprintf(os.Stdout, "Response from `MetricsApi.Ingest`: %v\n", resp)
}

func apiClient(url string) *openapiclient.APIClient {
	configuration := openapiclient.NewConfiguration()
	configuration.Servers = openapiclient.ServerConfigurations{
		openapiclient.ServerConfiguration{
			URL: url,
		},
	}
	api_client := openapiclient.NewAPIClient(configuration)
	return api_client
}

func (dynClient *DynatraceClientPoc) authorizedCtx(ctx context.Context) context.Context {
	return context.WithValue(
		ctx,
		openapiclient.ContextAPIKeys,
		map[string]openapiclient.APIKey{
			"Api-Token": {
				//TODO: load from secret
				Prefix: "Api-Token",
				Key:    dynClient.apiToken,
			},
		})
}

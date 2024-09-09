package clientoptl

import (
	"context"
	"testing"
)

func TestNewMetricClient_Real(t *testing.T) {
	t.Skip("skipping test")
	client, err := NewMetricClient("canary.eu21.apm.services.cloud.sap", "/e/1b9c6fb0-eb17-4fce-96b0-088cee0861b3/api/v2/", "dt0c01..")

	if err != nil {
		t.Errorf("Failed to create OTLP exporter: %v", err)
	}

	client.SetMeter("federated")

	mr, err := client.NewMetric("xmirza")

	if err != nil {
		t.Errorf("Failed to create metric: %v", err)
	}

	dp1 := NewDataPoint().AddDimension("location", "US").AddDimension("provider", "Azure").SetValue(3)
	dp2 := NewDataPoint().AddDimension("location", "EU").AddDimension("provider", "GCP").SetValue(5)
	dp3 := NewDataPoint().AddDimension("location", "AU").AddDimension("provider", "AWS").SetValue(7)

	err = mr.RecordMetrics(dp1, dp2, dp3)
	if err != nil {
		t.Errorf("Failed to record metrics: %v", err)
	}
	err = client.ExportMetrics(context.Background())
	if err != nil {
		t.Errorf("Failed to record metrics: %v", err)
	}
}

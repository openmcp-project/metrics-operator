package clientlite

import (
	"testing"
)

func TestSendMetric_Real(t *testing.T) {
	t.Skip("skipping test")
	dynatraceURL := "canary.eu21.apm.services.cloud.sap"
	apiToken := ""

	cl := NewMetricClient(dynatraceURL, "e/1b9c6fb0-eb17-4fce-96b0-088cee0861b3/api/v2", apiToken)

	mr := NewMetric("xmy.metric").
		AddDimension("device", "device1").
		AddDimension("location", "new_york").
		SetGaugeValue(10)

	mr2 := NewMetric("xmy.metric").
		AddDimension("device", "device2").
		AddDimension("location", "new_york").
		SetGaugeValue(20)

	mr3 := NewMetric("xmy.metric").
		AddDimension("device", "device3").
		AddDimension("location", "new_york").
		SetGaugeValue(12)

	err := cl.SendMetrics(mr, mr2, mr3)

	if err != nil {
		t.Errorf("Error sending metrics: %v\n", err)
	}
}

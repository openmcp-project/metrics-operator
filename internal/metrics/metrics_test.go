package metrics

import "testing"

func TestRecordDataPointAcceptsExplicitGVKLabels(t *testing.T) {
	RecordDataPoint("test_metric", "default", map[string]string{
		"target_kind":        "Pod",
		"target_group":       "n/a",
		"target_version":     "v1",
		"target_api_version": "v1",
		"cr_kind":            "Deployment",
		"cr_group":           "apps",
		"cr_version":         "v1",
		"cr_api_version":     "apps/v1",
		"cluster":            "cluster-a",
		"custom":             "value",
	}, 1)
}

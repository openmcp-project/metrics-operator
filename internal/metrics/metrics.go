package metrics

import (
	"encoding/json"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var ResourceCountGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "metrics_operator_resource_count",
		Help: "Count of Kubernetes resources observed by the metrics-operator.",
	},
	[]string{
		"metric_name",
		"namespace",
		"resource",
		"group",
		"version",
		"cluster",
		"kind",
		"api_version",
		"extra_labels",
	},
)

func init() {
	ctrlmetrics.Registry.MustRegister(ResourceCountGauge)
}

// RecordDataPoint records a single data point into ResourceCountGauge.
// metricName is the CR spec.Name, namespace is the CR namespace,
// dims is the DataPoint.Dimensions map, value is the gauge value.
func RecordDataPoint(metricName, namespace string, dims map[string]string, value int64) {
	fixed := map[string]string{
		"resource":    "",
		"group":       "",
		"version":     "",
		"cluster":     "",
		"kind":        "",
		"api_version": "",
	}
	overflow := make(map[string]string)
	for k, v := range dims {
		switch k {
		case "resource":
			fixed["resource"] = v
		case "group":
			fixed["group"] = v
		case "version":
			fixed["version"] = v
		case "cluster":
			fixed["cluster"] = v
		case "kind":
			fixed["kind"] = v
		case "apiVersion":
			fixed["api_version"] = v
		default:
			overflow[k] = v
		}
	}
	extra := "{}"
	if len(overflow) > 0 {
		if b, err := json.Marshal(overflow); err == nil {
			extra = string(b)
		}
	}
	ResourceCountGauge.With(prometheus.Labels{
		"metric_name": metricName,
		"namespace":   namespace,
		"resource":    fixed["resource"],
		"group":       fixed["group"],
		"version":     fixed["version"],
		"cluster":     fixed["cluster"],
		"kind":        fixed["kind"],
		"api_version": fixed["api_version"],
		"extra_labels": extra,
	}).Set(float64(value))
}

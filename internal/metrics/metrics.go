package metrics

import (
	"encoding/json"

	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	labelMetricName       = "metric_name"
	labelNamespace        = "namespace"
	labelTargetKind       = "target_kind"
	labelTargetGroup      = "target_group"
	labelTargetVersion    = "target_version"
	labelTargetAPIVersion = "target_api_version"
	labelCRKind           = "cr_kind"
	labelCRGroup          = "cr_group"
	labelCRVersion        = "cr_version"
	labelCRAPIVersion     = "cr_api_version"
	labelCluster          = "cluster"
	labelExtraLabels      = "extra_labels"
)

var ResourceCountGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "metrics_operator_resource_count",
		Help: "Count of Kubernetes resources observed by the metrics-operator.",
	},
	[]string{
		labelMetricName,
		labelNamespace,
		labelTargetKind,
		labelTargetGroup,
		labelTargetVersion,
		labelTargetAPIVersion,
		labelCRKind,
		labelCRGroup,
		labelCRVersion,
		labelCRAPIVersion,
		labelCluster,
		labelExtraLabels,
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
		labelTargetKind:       "",
		labelTargetGroup:      "",
		labelTargetVersion:    "",
		labelTargetAPIVersion: "",
		labelCRKind:           "",
		labelCRGroup:          "",
		labelCRVersion:        "",
		labelCRAPIVersion:     "",
		labelCluster:          "",
	}
	overflow := make(map[string]string)
	for k, v := range dims {
		switch k {
		case labelTargetKind:
			fixed[labelTargetKind] = v
		case labelTargetGroup:
			fixed[labelTargetGroup] = v
		case labelTargetVersion:
			fixed[labelTargetVersion] = v
		case labelTargetAPIVersion:
			fixed[labelTargetAPIVersion] = v
		case labelCRKind:
			fixed[labelCRKind] = v
		case labelCRGroup:
			fixed[labelCRGroup] = v
		case labelCRVersion:
			fixed[labelCRVersion] = v
		case labelCRAPIVersion, "cr_apiVersion":
			fixed[labelCRAPIVersion] = v
		case labelCluster:
			fixed[labelCluster] = v
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
		labelMetricName:       metricName,
		labelNamespace:        namespace,
		labelTargetKind:       fixed[labelTargetKind],
		labelTargetGroup:      fixed[labelTargetGroup],
		labelTargetVersion:    fixed[labelTargetVersion],
		labelTargetAPIVersion: fixed[labelTargetAPIVersion],
		labelCRKind:           fixed[labelCRKind],
		labelCRGroup:          fixed[labelCRGroup],
		labelCRVersion:        fixed[labelCRVersion],
		labelCRAPIVersion:     fixed[labelCRAPIVersion],
		labelCluster:          fixed[labelCluster],
		labelExtraLabels:      extra,
	}).Set(float64(value))
}

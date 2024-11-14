package client

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func Setup() MetricMetadata {
	return NewMetricMetadata("id", "name", "description")
}

func TestMetricAddDimension(t *testing.T) {
	tests := map[string]struct {
		input map[string]string
		want  map[string]string
	}{
		"one value":  {input: map[string]string{"Test": "0"}, want: map[string]string{"Test": "0"}},
		"two value":  {input: map[string]string{"key-one": "0", "key-two": "1"}, want: map[string]string{"key-one": "0", "key-two": "1"}},
		"zero value": {input: map[string]string{}, want: map[string]string{}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			// add dimensions
			for key, value := range tc.input {
				err := metric.AddDimension(key, value)
				if err != nil {
					t.Fatalf("an error when adding a dimension: %s", err)
				}
			}

			diff := cmp.Diff(tc.want, metric.Metric.dimensions)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricAddDimensions(t *testing.T) {
	tests := map[string]struct {
		input map[string]string
		want  map[string]string
	}{
		"one value":  {input: map[string]string{"Test": "0"}, want: map[string]string{"Test": "0"}},
		"two value":  {input: map[string]string{"key-one": "0", "key-two": "1"}, want: map[string]string{"key-one": "0", "key-two": "1"}},
		"zero value": {input: map[string]string{}, want: map[string]string{}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			// add dimensions
			err := metric.AddDimensions(tc.input)
			if err != nil {
				t.Fatalf("an error when adding a dimension: %s", err)
			}

			diff := cmp.Diff(tc.want, metric.Metric.dimensions)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricClearDimensions(t *testing.T) {
	tests := map[string]struct {
		input map[string]string
		want  map[string]string
	}{
		"one value":  {input: map[string]string{"Test": "0"}, want: map[string]string{}},
		"two value":  {input: map[string]string{"key-one": "0", "key-two": "1"}, want: map[string]string{}},
		"zero value": {input: map[string]string{}, want: map[string]string{}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			// add dimensions
			for key, value := range tc.input {
				err := metric.AddDimension(key, value)
				if err != nil {
					t.Fatalf("an error when adding a dimension: %s", err)
				}
			}

			metric.ClearDimensions()
			diff := cmp.Diff(tc.want, metric.Metric.dimensions)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricAddDatapoint(t *testing.T) {
	tests := map[string]struct {
		input []float64
		want  []float64
	}{
		"one value":  {input: []float64{0}, want: []float64{0}},
		"two value":  {input: []float64{2.42, 6.69}, want: []float64{2.42, 6.69}},
		"zero value": {input: []float64{}, want: []float64{}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			// add datapoint
			for _, value := range tc.input {
				metric.AddDatapoint(value)
			}

			diff := cmp.Diff(tc.want, metric.Metric.datapoints)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricAddDatapoints(t *testing.T) {
	tests := map[string]struct {
		input []float64
		want  []float64
	}{
		"one value":  {input: []float64{0}, want: []float64{0}},
		"two value":  {input: []float64{2.42, 6.69}, want: []float64{2.42, 6.69}},
		"zero value": {input: []float64{}, want: []float64{}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			// add datapoints
			metric.AddDatapoints(tc.input...)

			diff := cmp.Diff(tc.want, metric.Metric.datapoints)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricClearDatapoints(t *testing.T) {
	tests := map[string]struct {
		input []float64
		want  []float64
	}{
		"one value":  {input: []float64{0}, want: []float64{}},
		"two value":  {input: []float64{2.42, 6.69}, want: []float64{}},
		"zero value": {input: []float64{}, want: []float64{}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			// add datapoints
			metric.AddDatapoints(tc.input...)

			metric.ClearDatapoints()
			diff := cmp.Diff(tc.want, metric.Metric.datapoints)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricSetTimestamp(t *testing.T) {
	tests := map[string]struct {
		input int64
		want  int64
	}{
		"now":  {input: time.Now().UnixMilli(), want: time.Now().UnixMilli()},
		"zero": {input: 0, want: 0},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			// set type
			err := metric.SetTimestamp(tc.input)
			if err != nil {
				t.Fatalf("timestamp was not set on the object: %s", err)
			}

			diff := cmp.Diff(tc.want, metric.Metric.timestamp)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricSetTypeCount(t *testing.T) {
	tests := map[string]struct {
		input []float64
		want  MetricType
	}{
		"one":   {input: []float64{2.42}, want: COUNT},
		"three": {input: []float64{2.42, 42.0, 6.90}, want: GAUGE},
		"zero":  {input: []float64{}, want: GAUGE},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			// set type
			metric.AddDatapoints(tc.input...)
			_ = metric.SetTypeCount(10)

			diff := cmp.Diff(tc.want, metric.Metric.valueType)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricSetUnit(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"one":   {input: "Percent", want: "Percent"},
		"three": {input: "kg", want: "kg"},
		"zero":  {input: "", want: ""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			// set unit
			metric.SetUnit(tc.input)

			diff := cmp.Diff(tc.want, metric.value.Unit)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricAddTags(t *testing.T) {
	tests := map[string]struct {
		input []string
		want  []string
	}{
		"three": {input: []string{"super", "cool", "tags"}, want: []string{"super", "cool", "tags"}},
		"one":   {input: []string{"tags"}, want: []string{"tags"}},
		"zero":  {input: []string{}, want: []string{}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			// set tags
			metric.AddTags(tc.input)

			diff := cmp.Diff(tc.want, metric.value.Tags)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricAddTag(t *testing.T) {
	tests := map[string]struct {
		input []string
		want  []string
	}{
		"three": {input: []string{"super", "cool", "tags"}, want: []string{"super", "cool", "tags"}},
		"one":   {input: []string{"tags"}, want: []string{"tags"}},
		"zero":  {input: []string{}, want: []string{}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			// set tags
			for _, tag := range tc.input {
				metric.AddTag(tag)
			}

			diff := cmp.Diff(tc.want, metric.value.Tags)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricSetMaxValue(t *testing.T) {
	tests := map[string]struct {
		input float64
		want  float64
	}{
		"one":  {input: 64.42, want: 64.42},
		"zero": {input: 0.0, want: 0.0},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			// set tags

			metric.SetMaxValue(tc.input)

			diff := cmp.Diff(tc.want, metric.value.MetricProperties.MaxValue)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricSetMinValue(t *testing.T) {
	tests := map[string]struct {
		input float64
		want  float64
	}{
		"one":  {input: 64.42, want: 64.42},
		"zero": {input: 0.0, want: 0.0},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			// set tags

			metric.SetMinValue(tc.input)

			diff := cmp.Diff(tc.want, metric.value.MetricProperties.MinValue)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricSetRootCauseRelevant(t *testing.T) {
	tests := map[string]struct {
		input bool
		want  bool
	}{
		"true":  {input: true, want: true},
		"false": {input: false, want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			metric.SetRootCauseRelevant(tc.input)

			diff := cmp.Diff(tc.want, metric.value.MetricProperties.RootCauseRelevant)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricSetImpactRelevant(t *testing.T) {
	tests := map[string]struct {
		input bool
		want  bool
	}{
		"true":  {input: true, want: true},
		"false": {input: false, want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			metric.SetImpactRelevant(tc.input)

			diff := cmp.Diff(tc.want, metric.value.MetricProperties.ImpactRelevant)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricSetValueType(t *testing.T) {
	tests := map[string]struct {
		input ValueType
		want  ValueType
	}{
		"score": {input: SCORE, want: SCORE},
		"error": {input: ERROR, want: ERROR},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			metric.SetValueType(tc.input)

			diff := cmp.Diff(tc.want, metric.value.MetricProperties.ValueType)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

func TestMetricSetLatency(t *testing.T) {
	tests := map[string]struct {
		input int
		want  int
	}{
		"positive": {input: 20, want: 20},
		"zero":     {input: 0, want: 0},
		"negative": {input: -13, want: 0},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			metric.SetLatency(tc.input)

			diff := cmp.Diff(tc.want, metric.value.MetricProperties.Latency)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}
func TestMetricAddMetadataDimension(t *testing.T) {
	tests := map[string]struct {
		input []Dimension
		want  []Dimension
	}{
		"one":      {input: []Dimension{{Key: "dimension", DisplayName: "display name"}}, want: []Dimension{{Key: "dimension", DisplayName: "display name"}}},
		"zero":     {input: []Dimension{}, want: []Dimension{}},
		"multiple": {input: []Dimension{{Key: "dimension", DisplayName: "display name"}, {Key: "dimension-two", DisplayName: "display name two"}}, want: []Dimension{{Key: "dimension", DisplayName: "display name"}, {Key: "dimension-two", DisplayName: "display name two"}}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			metric := Setup()

			for _, dimensions := range tc.input {
				metric.AddMetadataDimension(dimensions.Key, dimensions.DisplayName)
			}

			diff := cmp.Diff(tc.want, metric.value.Dimensions)
			if diff != "" {
				t.Fatalf("%s", diff)
			}
		})
	}
}

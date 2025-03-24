package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	dyn "github.com/dynatrace-ace/dynatrace-go-api-client/api/v2/environment/dynatrace"
)

// MetricType is used to define the type of metric that is being sent to the backend
type MetricType string

// ValueType is used to define the type of value that is being sent to the backend
type ValueType string

const ( // defines the two types of MetricTypes there are in Dynatrace
	// GAUGE is used to send a single value to the backend
	GAUGE MetricType = "gauge"
	// COUNT is used to send a single value to the backend
	COUNT MetricType = "count"
)

const ( // defines the two types of MetricTypes there are in Dynatrace
	// SCORE is used to send a single value to the backend
	SCORE ValueType = "score"
	// ERROR is used to send a single value to the backend
	ERROR ValueType = "error"
)

// Metric The metric is holds all information for creating a metric on the dynatrace end
// there is no metadata fields
type Metric struct {
	id         *string
	dimensions map[string]string
	datapoints []float64
	timestamp  int64 // UTC Milliseconds Timestamp
	valueType  MetricType
	Min        float64
	max        float64
	sum        float64
	count      uint64
	delta      float64
}

// MetricMetadata This struct combines the raw metric and the metadata to generate and use both as a whole package
type MetricMetadata struct {
	Metric  Metric
	Setting dyn.SettingsObjectCreate
	value   SettingsValue
}

// SettingsValue This holds all the metadata, naming is adjusted to dynatrace objects
type SettingsValue struct {
	DisplayName      string           `json:"displayName,omitempty"`
	Description      string           `json:"description,omitempty"`
	Unit             string           `json:"unit,omitempty"`
	Tags             []string         `json:"tags,omitempty"`
	MetricProperties MetricProperties `json:"metricProperties,omitempty"`
	Dimensions       []Dimension      `json:"dimensions,omitempty"`
}

// MetricProperties holds more specific parts of the dynatrace metadata
type MetricProperties struct {
	MaxValue          float64   `json:"maxValue,omitempty"`
	MinValue          float64   `json:"minValue,omitempty"`
	RootCauseRelevant bool      `json:"rootCauseRelevant,omitempty"`
	ImpactRelevant    bool      `json:"impactRelevant,omitempty"`
	ValueType         ValueType `json:"valueType,omitempty"`
	Latency           int       `json:"latency,omitempty"`
}

// Dimension this is used to hold dimensions for the metadata which dont need a concrete value, just a name and an id
type Dimension struct {
	Key         string `json:"key,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

// NewMetricMetadata Create a new MetricMetadata object which can be send to dynatrace via the dynatrace client in the same module
//
// metricId: an identifier for your metric (should be unique)
//
// displayName: will be applied with the metadata of the metric
//
// description: will be applied with the metadata of the metric
func NewMetricMetadata(metricID string, displayName string, description string) MetricMetadata {
	m := MetricMetadata{
		Metric: Metric{
			id:         &metricID,
			count:      0,
			dimensions: make(map[string]string),
			datapoints: []float64{},
			valueType:  GAUGE,
		},
		value: SettingsValue{
			MetricProperties: MetricProperties{
				Latency:           0,
				ValueType:         SCORE,
				ImpactRelevant:    false,
				RootCauseRelevant: false,
			},
			DisplayName: displayName,
			Description: description,
			Tags:        []string{},
			Dimensions:  []Dimension{},
		},
		Setting: dyn.SettingsObjectCreate{
			Scope:    "metric-" + metricID,
			SchemaId: "builtin:metric.metadata",
		},
	}

	return m
}

// AddDimension Add a dimension with a concrete value to the metric
func (m *MetricMetadata) AddDimension(key string, value string) error {
	_, found := m.Metric.dimensions[key]
	if !found {
		m.Metric.dimensions[key] = value
	} else {
		return fmt.Errorf("key %s already exists", key)
	}
	return nil
}

// AddDimensions Add multiple dimensions with concrete values to the metric
func (m *MetricMetadata) AddDimensions(dimension map[string]string) error {
	for key, value := range dimension {
		_, found := m.Metric.dimensions[key]
		if !found {
			m.Metric.dimensions[key] = value
		} else {
			return fmt.Errorf("key %s already exists", key)
		}
	}

	return nil
}

// RemoveDimension remove dimension by key
func (m *MetricMetadata) RemoveDimension(key string) {
	delete(m.Metric.dimensions, key)
}

// ClearDimensions empty all dimensions and replaces them with an empty map
func (m *MetricMetadata) ClearDimensions() {
	m.Metric.dimensions = map[string]string{}
}

// AddDatapoint Add a datapoint that will be the value displayed on the graph in dynatrace
// This function only appends to a list of datapoints.
// The payload if multiple datapoints exist consist of multiple statistics calculated from the list of points.
// Payload consists of: minimum, maximum, count and sum
//
// Type regarding the number of Datapoints:
// When Adding more than one datapoint the Type of the Metric will automatically be set to GAUGE
//
// If there is only one datapoint the type will still be GAUGE, but can be manually set to COUNT via "SetValueType()"
func (m *MetricMetadata) AddDatapoint(point float64) {
	m.Metric.datapoints = append(m.Metric.datapoints, point)
	m.Metric.Min = slices.Min(m.Metric.datapoints)
	m.Metric.max = slices.Max(m.Metric.datapoints)
	m.Metric.count = uint64(len(m.Metric.datapoints))
	m.Metric.valueType = GAUGE

	for _, value := range m.Metric.datapoints {
		m.Metric.sum += value
	}
}

// AddDatapoints Add multiple datapoints that will be the value displayed on the graph in dynatrace
// This function only appends to a list of datapoints.
// The payload if multiple datapoints exist consist of multiple statistics calculated from the list of points.
// Payload consists of: minimum, maximum, count and sum
//
// Type regarding the number of Datapoints:
// When Adding more than one datapoint the Type of the Metric will automatically be set to GAUGE
//
// If there is only one datapoint the type will still be GAUGE, but can be manually set to COUNT via "SetValueType()"
func (m *MetricMetadata) AddDatapoints(points ...float64) {
	if len(points) == 0 {
		return
	}
	m.Metric.datapoints = append(m.Metric.datapoints, points...)
	m.Metric.Min = slices.Min(m.Metric.datapoints)
	m.Metric.max = slices.Max(m.Metric.datapoints)
	m.Metric.count = uint64(len(m.Metric.datapoints))
	m.Metric.valueType = GAUGE
	for _, value := range m.Metric.datapoints {
		m.Metric.sum += value
	}
}

// ClearDatapoints replaces list of datapoints with an empty array
func (m *MetricMetadata) ClearDatapoints() {
	m.Metric.datapoints = []float64{}
}

// SetTimestamp Set the timestamp of the metric manually
// if not set the timestamp when the metric is received will be used
//
// Rules:
//
// - Up to 10 minutes in the future
//
// - Up to 1 hour in the past
func (m *MetricMetadata) SetTimestamp(timestamp int64) error {
	pastLimit := int64(time.Now().Add(time.Hour*-1).UTC().Nanosecond()) / int64(time.Millisecond)
	futureLimit := int64(time.Now().Add(time.Minute*10).UTC().Nanosecond()) / int64(time.Millisecond)
	if timestamp > pastLimit || timestamp < futureLimit {
		m.Metric.timestamp = timestamp
		return nil
	}
	return errors.New("timestamp can only be 10 minutes into the future or 1 hour into the past")
}

// SetTypeCount Set type to COUNT
//
// Type count can only be set if one datapoint exists
// the reason being, that count will add a delta to the current datapoint and records this in dynatrace
func (m *MetricMetadata) SetTypeCount(delta float64) error {
	if len(m.Metric.datapoints) == 1 {
		m.Metric.valueType = COUNT
		m.Metric.delta = delta
		return nil
	}
	return fmt.Errorf("using type count is only possible with one datapoint: current count %v", len(m.Metric.datapoints))
}

// SetUnit Set the unit of measure for your metric
//
// This is also only send via metric metadata
func (m *MetricMetadata) SetUnit(unit string) {
	m.value.Unit = unit
}

// AddTags Add tags for your metric
//
// # This is also only send via metric metadata
//
// This can be used to filter for your metric in the dynatrace UI
func (m *MetricMetadata) AddTags(tags []string) {
	m.value.Tags = append(m.value.Tags, tags...)

}

// AddTag Add a tag to your metric
//
// # This is also only send via metric metadata
//
// This can be used to filter for your metric in the dynatrace UI
func (m *MetricMetadata) AddTag(tag string) {
	m.value.Tags = append(m.value.Tags, tag)

}

// SetMaxValue Add a maximum value to your metric
//
// # This is also only send via metric metadata
//
// This can be used to limit the value (useful if a metric is used that sums up over a long period of time)
func (m *MetricMetadata) SetMaxValue(value float64) {
	m.value.MetricProperties.MaxValue = value

}

// SetMinValue Add a minimum value to your metric
//
// # This is also only send via metric metadata
//
// This can be used to limit the value (useful if a metric is used that sums up over a long period of time)
func (m *MetricMetadata) SetMinValue(value float64) {
	m.value.MetricProperties.MinValue = value

}

// SetRootCauseRelevant Add a rootcause relevant flag to your metric
//
// # This is also only send via metric metadata
//
// This can be used to filter for your metric
func (m *MetricMetadata) SetRootCauseRelevant(value bool) {
	m.value.MetricProperties.RootCauseRelevant = value

}

// SetImpactRelevant Add a impact relevant flag to your metric
//
// # This is also only send via metric metadata
//
// This can be used to filter for your metric
func (m *MetricMetadata) SetImpactRelevant(value bool) {
	m.value.MetricProperties.ImpactRelevant = value

}

// SetValueType Set the Value type of your metric
//
// # This is also only send via metric metadata
//
// This can be used to filter for your metric
func (m *MetricMetadata) SetValueType(value ValueType) {
	m.value.MetricProperties.ValueType = value

}

// SetLatency Set the latency of your metric
//
// This is also only send via metric metadata
func (m *MetricMetadata) SetLatency(latency int) {
	if latency > 0 {
		m.value.MetricProperties.Latency = latency
	}

}

// AddMetadataDimension Add a dimension to your metric
//
// # This is also only send via metric metadata
//
// This can be used to predefine the available metrics without sending a datapoint
func (m *MetricMetadata) AddMetadataDimension(key string, displayName string) {
	if key != "" && displayName != "" {
		m.value.Dimensions = append(m.value.Dimensions, Dimension{Key: key, DisplayName: displayName})
	}

}

// AddMetadataDimensions Add dimensions to your metric
//
// # This is also only send via metric metadata
//
// This can be used to predefine the available metrics without sending a datapoint
func (m *MetricMetadata) AddMetadataDimensions(dimensions []Dimension) {
	m.value.Dimensions = append(m.value.Dimensions, dimensions...)

}

// GenerateMetricBody Generate the payload body that can be used to send a datapoint with dimensions to the backend
//
// This will generate a body to send a datapoint to the backend (please use the client-api if you want to send something)
// This can be used to check if the body is correctly generated
func (m *MetricMetadata) GenerateMetricBody() string {
	// format: metric.key,dimensions format,datapoint,timestamp
	dimensionsString := ","
	for key, value := range m.Metric.dimensions {
		dimensionsString += key + "=" + value + ","
	}
	dimensionsString = strings.TrimSuffix(dimensionsString, ",")

	formatString := string(m.Metric.valueType)

	var datapointString string
	if m.Metric.valueType == COUNT {
		datapointString += "delta=" + strconv.FormatFloat(m.Metric.delta, 'f', 2, 64)
	} else {
		datapointString += "min=" + strconv.FormatFloat(m.Metric.Min, 'f', 2, 64) + ",max=" + strconv.FormatFloat(m.Metric.Min, 'f', 2, 64) + ",sum=" + strconv.FormatFloat(m.Metric.Min, 'f', 2, 64) + ",count=" + fmt.Sprint(len(m.Metric.datapoints))
	}

	body := *m.Metric.id + dimensionsString + " " + formatString + "," + datapointString

	return body
}

// GenerateSettingsObjects Generate the payload body that can be used to send metric metadata to the backend
//
// This will generate a SettingsObject to send metadata for a sepcific metric to the backend (PLEASE use the client-api if you want to send something)
// This can be used to check if the body is correctly generated
func (m *MetricMetadata) GenerateSettingsObjects() ([]dyn.SettingsObjectCreate, error) {
	val := m.value
	js, err := json.Marshal(val)
	if err != nil {
		return []dyn.SettingsObjectCreate{}, err
	}
	var mapped map[string]interface{}
	err = json.Unmarshal(js, &mapped)
	if err != nil {
		return []dyn.SettingsObjectCreate{}, err
	}
	m.Setting.Value = mapped

	return []dyn.SettingsObjectCreate{m.Setting}, nil
}

// GenerateUpdateSettings Generate the payload body that can be used to send metric metadata to the backend
//
// This will generate a SettingsObjectUpdate to send updated metadata for a sepcific metric to the backend (PLEASE use the client-api if you want to send something)
// This can be used to check if the body is correctly generated
func (m *MetricMetadata) GenerateUpdateSettings(_ string, metric MetricMetadata, updateToken string) (dyn.SettingsObjectUpdate, error) {
	val := metric.value
	js, err := json.Marshal(val)
	if err != nil {
		return dyn.SettingsObjectUpdate{}, err
	}
	var mapped map[string]interface{}
	err = json.Unmarshal(js, &mapped)
	if err != nil {
		return dyn.SettingsObjectUpdate{}, err
	}
	var setting dyn.SettingsObjectUpdate

	if updateToken != "" {
		setting = dyn.SettingsObjectUpdate{UpdateToken: &updateToken, Value: mapped}
		return setting, nil
	}
	setting = dyn.SettingsObjectUpdate{Value: mapped}
	return setting, nil

}

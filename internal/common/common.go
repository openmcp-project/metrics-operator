package common

// DataSinkCredentials holds the credentials to access the data sink (e.g. dynatrace)
type DataSinkCredentials struct {
	Host  string
	Path  string
	Token string
}

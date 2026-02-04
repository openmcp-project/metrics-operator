package common

// DataSinkCredentials holds the credentials to access the data sink
type DataSinkCredentials struct {
	Host string
	Path string

	// Token-based authentication
	APIKey *APIKeyAuth

	// Certificate-based authentication (mutual TLS)
	Certificate *CertificateAuth
}

type APIKeyAuth struct {
	Token string
}

type CertificateAuth struct {
	ClientCert []byte
	ClientKey  []byte
	CACert     []byte
}

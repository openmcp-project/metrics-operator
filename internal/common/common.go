package common

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
)

const (
	// SecretNameSpace is the namespace where the secret is deployed
	SecretNameSpace string = "metrics-operator"
	// SecretName is the name of the secret
	SecretName string = "co-dynatrace-credentials"
)

// GetCredentialsSecret Get the Secret with access token from the cluster, which you deployed earlier into the system
//
// Deployment of secret:
//
// you can deploy the metric through: kubectl apply -f sample/secret.yaml
func GetCredentialsSecret(ctx context.Context, client client.Client) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	namespace := types.NamespacedName{
		Namespace: SecretNameSpace,
		Name:      SecretName,
	}
	err := client.Get(ctx, namespace, secret)
	if err != nil {
		return &corev1.Secret{}, err
	}
	return secret, nil
}

// GetCredentialData returns the data from the secret
func GetCredentialData(secret *corev1.Secret) DataSinkCredentials {
	creds := DataSinkCredentials{
		Host:  string(secret.Data["Host"]),
		Path:  string(secret.Data["Path"]),
		Token: string(secret.Data["Token"]),
	}
	return creds
}

// DataSinkCredentials holds the credentials to access the data sink (e.g. dynatrace)
type DataSinkCredentials struct {
	Host  string
	Path  string
	Token string
}

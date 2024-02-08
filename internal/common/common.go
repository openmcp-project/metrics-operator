package common

import (
	"context"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
)

const (
	SecretNameSpace string = "co-metrics-operator"
	SecretName      string = "co-dynatrace-credentials"
)

// Get the Secret with access token from the cluster, which you deployed earlier into the system
//
// Deployment of secret:
//
// you can deploy the metric through: kubectl apply -f sample/secret.yaml
func GetClusterSecret(client client.Client, ctx context.Context) (*corev1.Secret, error) {
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

/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"github.com/openmcp-project/metrics-operator/internal/common"
)

// DataSinkCredentialsRetriever provides common functionality for retrieving DataSink credentials
type DataSinkCredentialsRetriever struct {
	client   client.Client
	recorder events.EventRecorder
}

// NewDataSinkCredentialsRetriever creates a new DataSinkCredentialsRetriever
func NewDataSinkCredentialsRetriever(client client.Client, recorder events.EventRecorder) *DataSinkCredentialsRetriever {
	return &DataSinkCredentialsRetriever{
		client:   client,
		recorder: recorder,
	}
}

// GetDataSinkCredentials fetches DataSink configuration and credentials for any metric type
//
//nolint:gocyclo
func (d *DataSinkCredentialsRetriever) GetDataSinkCredentials(ctx context.Context, dataSinkRef *v1alpha1.DataSinkReference, eventObject client.Object, l logr.Logger) (common.DataSinkCredentials, error) {

	// Determine the namespace where DataSinks are expected to be found.
	dataSinkLookupNamespace := os.Getenv("OPERATOR_CONFIG_NAMESPACE")
	if dataSinkLookupNamespace == "" {
		l.V(1).Info("OPERATOR_CONFIG_NAMESPACE not set, trying POD_NAMESPACE.")
		dataSinkLookupNamespace = os.Getenv("POD_NAMESPACE")
		if dataSinkLookupNamespace == "" {
			l.Info("Neither OPERATOR_CONFIG_NAMESPACE nor POD_NAMESPACE is set. Defaulting DataSink lookup to 'default' namespace.")
			dataSinkLookupNamespace = "default"
		} else {
			l.Info("Using POD_NAMESPACE for DataSink lookup.", "namespace", dataSinkLookupNamespace)
		}
	} else {
		l.Info("Using OPERATOR_CONFIG_NAMESPACE for DataSink lookup.", "namespace", dataSinkLookupNamespace)
	}

	// Determine DataSink name
	dataSinkName := "default"
	if dataSinkRef != nil && dataSinkRef.Name != "" {
		dataSinkName = dataSinkRef.Name
	}

	// Fetch DataSink CR
	dataSink := &v1alpha1.DataSink{}
	dataSinkKey := types.NamespacedName{
		Namespace: dataSinkLookupNamespace,
		Name:      dataSinkName,
	}

	if err := d.client.Get(ctx, dataSinkKey, dataSink); err != nil {
		if apierrors.IsNotFound(err) {
			l.Error(err, fmt.Sprintf("DataSink '%s' not found in namespace '%s'", dataSinkName, dataSinkLookupNamespace))
			d.recorder.Eventf(eventObject, nil, "Error", "DataSinkNotFound", "GetDataSinkCredentials", fmt.Sprintf("DataSink '%s' not found in namespace '%s'", dataSinkName, dataSinkLookupNamespace))
		} else {
			l.Error(err, fmt.Sprintf("unable to fetch DataSink '%s' in namespace '%s'", dataSinkName, dataSinkLookupNamespace))
			d.recorder.Eventf(eventObject, nil, "Error", "DataSinkFetchError", "GetDataSinkCredentials", fmt.Sprintf("unable to fetch DataSink '%s' in namespace '%s'", dataSinkName, dataSinkLookupNamespace))
		}
		return common.DataSinkCredentials{}, err
	}

	// Extract endpoint from DataSink
	endpoint := dataSink.Spec.Connection.Endpoint
	// Construct credentials compatible with clientoptl.NewMetricClient
	// For now, we'll use the full endpoint as Host and empty Path
	// TODO: Parse endpoint to separate host and path if needed based on protocol
	credentials := common.DataSinkCredentials{
		Host: endpoint, // Full endpoint URL (e.g., https://example.dynatrace.com)
		Path: "",       // Base path for API (will be combined with /otlp/v1/metrics in clientoptl)
	}

	// Handle token authentication
	var token string
	if dataSink.Spec.Authentication != nil && dataSink.Spec.Authentication.APIKey != nil {
		// Fetch credentials secret
		secretName := dataSink.Spec.Authentication.APIKey.SecretKeyRef.Name
		secretKey := dataSink.Spec.Authentication.APIKey.SecretKeyRef.Key

		secret := &corev1.Secret{}
		secretNamespacedName := types.NamespacedName{
			Namespace: dataSinkLookupNamespace,
			Name:      secretName,
		}

		if err := fetchSecret(ctx, d.client, secretNamespacedName, secret, l); err != nil {
			return common.DataSinkCredentials{}, err
		}

		// Extract token from secret
		tokenBytes, err := getSecretKeyData(secret, secretKey, secretName, l)
		if err != nil {
			d.recorder.Eventf(eventObject, nil, "Error", "SecretKeyNotFound", "GetDataSinkCredentials", fmt.Sprintf("key '%s' not found in secret '%s'", secretKey, secretName))
			return common.DataSinkCredentials{}, err
		}
		token = string(tokenBytes)

		credentials.APIKey = &common.APIKeyAuth{
			Token: token,
		}
	}

	// Handle certificate-based authentication
	if dataSink.Spec.Authentication != nil && dataSink.Spec.Authentication.Certificate != nil {
		secretNameClientCert := dataSink.Spec.Authentication.Certificate.ClientCert.Name
		secretKeyClientCert := dataSink.Spec.Authentication.Certificate.ClientCert.Key

		secretNameClientKey := dataSink.Spec.Authentication.Certificate.ClientKey.Name
		secretKeyClientKey := dataSink.Spec.Authentication.Certificate.ClientKey.Key

		secret := &corev1.Secret{}
		secretNamespacedName := types.NamespacedName{
			Namespace: dataSinkLookupNamespace,
			Name:      secretNameClientCert,
		}

		if err := fetchSecret(ctx, d.client, secretNamespacedName, secret, l); err != nil {
			return common.DataSinkCredentials{}, err
		}

		clientCert, err := getSecretKeyData(secret, secretKeyClientCert, secretNameClientCert, l)
		if err != nil {
			d.recorder.Eventf(eventObject, nil, "Error", "SecretKeyNotFound", "GetDataSinkCredentials", fmt.Sprintf("key '%s' not found in secret '%s'", secretKeyClientCert, secretNameClientCert))
			return common.DataSinkCredentials{}, err
		}

		clientKey, err := getSecretKeyData(secret, secretKeyClientKey, secretNameClientKey, l)
		if err != nil {
			d.recorder.Eventf(eventObject, nil, "Error", "SecretKeyNotFound", "GetDataSinkCredentials", fmt.Sprintf("key '%s' not found in secret '%s'", secretKeyClientKey, secretNameClientKey))
			return common.DataSinkCredentials{}, err
		}

		credentials.Certificate = &common.CertificateAuth{
			ClientCert: clientCert,
			ClientKey:  clientKey,
		}

		secretNamespacedName.Name = secretNameClientKey
		if err = fetchSecret(ctx, d.client, secretNamespacedName, secret, l); err != nil {
			return common.DataSinkCredentials{}, err
		}

		if dataSink.Spec.Authentication.Certificate.CACert != nil {
			secretNameCACert := dataSink.Spec.Authentication.Certificate.CACert.Name
			secretKeyCACert := dataSink.Spec.Authentication.Certificate.CACert.Key

			secretNamespacedName.Name = secretNameCACert
			if err := d.client.Get(ctx, secretNamespacedName, secret); err != nil {
				return common.DataSinkCredentials{}, err
			}

			credentials.Certificate.CACert, err = getSecretKeyData(secret, secretKeyCACert, secretNameCACert, l)
			if err != nil {
				d.recorder.Eventf(eventObject, nil, "Error", "SecretKeyNotFound", "GetDataSinkCredentials", fmt.Sprintf("key '%s' not found in secret '%s'", secretKeyCACert, secretNameCACert))
				return common.DataSinkCredentials{}, err
			}
		}
	}

	l.Info(fmt.Sprintf("Using DataSink '%s' with endpoint '%s'", dataSinkName, endpoint))

	return credentials, nil
}

func fetchSecret(ctx context.Context, c client.Client, namespacedName types.NamespacedName, secret *corev1.Secret, l logr.Logger) error {
	if err := c.Get(ctx, namespacedName, secret); err != nil {
		if apierrors.IsNotFound(err) {
			l.Error(err, fmt.Sprintf("Secret '%s' not found in namespace '%s'", namespacedName.Name, namespacedName.Namespace))
		} else {
			l.Error(err, fmt.Sprintf("unable to fetch Secret '%s' in namespace '%s'", namespacedName.Name, namespacedName.Namespace))
		}
		return err
	}
	return nil
}

func getSecretKeyData(secret *corev1.Secret, key string, secretName string, l logr.Logger) ([]byte, error) {
	value, exists := secret.Data[key]
	if !exists {
		err := fmt.Errorf("key '%s' not found in secret '%s'", key, secretName)
		l.Error(err, fmt.Sprintf("key '%s' not found in secret '%s'", key, secretName))
		return nil, err
	}
	return value, nil
}

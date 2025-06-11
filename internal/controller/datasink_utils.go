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
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openmcp-project/metrics-operator/api/v1alpha1"
	"github.com/openmcp-project/metrics-operator/internal/common"
)

// DataSinkCredentialsRetriever provides common functionality for retrieving DataSink credentials
type DataSinkCredentialsRetriever struct {
	client   client.Client
	recorder record.EventRecorder
}

// NewDataSinkCredentialsRetriever creates a new DataSinkCredentialsRetriever
func NewDataSinkCredentialsRetriever(client client.Client, recorder record.EventRecorder) *DataSinkCredentialsRetriever {
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
			d.recorder.Event(eventObject, "Error", "DataSinkNotFound", fmt.Sprintf("DataSink '%s' not found in namespace '%s'", dataSinkName, dataSinkLookupNamespace))
		} else {
			l.Error(err, fmt.Sprintf("unable to fetch DataSink '%s' in namespace '%s'", dataSinkName, dataSinkLookupNamespace))
			d.recorder.Event(eventObject, "Error", "DataSinkFetchError", fmt.Sprintf("unable to fetch DataSink '%s' in namespace '%s'", dataSinkName, dataSinkLookupNamespace))
		}
		return common.DataSinkCredentials{}, err
	}

	// Extract endpoint from DataSink
	endpoint := dataSink.Spec.Connection.Endpoint

	// Handle authentication
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

		if err := d.client.Get(ctx, secretNamespacedName, secret); err != nil {
			if apierrors.IsNotFound(err) {
				l.Error(err, fmt.Sprintf("Secret '%s' not found in namespace '%s'", secretName, dataSinkLookupNamespace))
				d.recorder.Event(eventObject, "Error", "SecretNotFound", fmt.Sprintf("Secret '%s' not found in namespace '%s'", secretName, dataSinkLookupNamespace))
			} else {
				l.Error(err, fmt.Sprintf("unable to fetch Secret '%s' in namespace '%s'", secretName, dataSinkLookupNamespace))
				d.recorder.Event(eventObject, "Error", "SecretFetchError", fmt.Sprintf("unable to fetch Secret '%s' in namespace '%s'", secretName, dataSinkLookupNamespace))
			}
			return common.DataSinkCredentials{}, err
		}

		// Extract token from secret
		tokenBytes, exists := secret.Data[secretKey]
		if !exists {
			err := fmt.Errorf("key '%s' not found in secret '%s'", secretKey, secretName)
			l.Error(err, fmt.Sprintf("key '%s' not found in secret '%s'", secretKey, secretName))
			d.recorder.Event(eventObject, "Error", "SecretKeyNotFound", fmt.Sprintf("key '%s' not found in secret '%s'", secretKey, secretName))
			return common.DataSinkCredentials{}, err
		}
		token = string(tokenBytes)
	}

	// Construct credentials compatible with clientoptl.NewMetricClient
	// The NewMetricClient expects: dtAPIHost, dtAPIBasePath, dtAPIToken
	// For now, we'll use the full endpoint as Host and empty Path
	// TODO: Parse endpoint to separate host and path if needed based on protocol
	credentials := common.DataSinkCredentials{
		Host:  endpoint, // Full endpoint URL (e.g., https://example.dynatrace.com)
		Path:  "",       // Base path for API (will be combined with /otlp/v1/metrics in clientoptl)
		Token: token,
	}

	l.Info(fmt.Sprintf("Using DataSink '%s' with endpoint '%s'", dataSinkName, endpoint))

	return credentials, nil
}

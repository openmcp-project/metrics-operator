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
	"testing"

	"github.com/stretchr/testify/require"

	"k8s.io/client-go/rest"
)

func TestGetClusterInfo(t *testing.T) {
	testCases := []struct {
		name           string
		host           string
		expectedName   string
		expectedError  bool
		errorSubstring string
	}{
		{
			name:          "EmptyHost",
			host:          "",
			expectedError: true,
		},
		{
			name:         "LocalhostIP",
			host:         "https://127.0.0.1:6443",
			expectedName: "localhost",
		},
		{
			name:         "KubernetesService",
			host:         "https://kubernetes.default.svc:6443",
			expectedName: "kubernetes",
		},
		{
			name:         "CustomClusterName",
			host:         "https://my-cluster-api.example.com:6443",
			expectedName: "my-cluster-api",
		},
		{
			name:         "IPAddress",
			host:         "https://192.168.1.1:6443",
			expectedName: "192", // The function only extracts the first part of the IP address
		},
		{
			name:         "WithPath",
			host:         "https://kubernetes.default.svc:6443/api",
			expectedName: "kubernetes",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &rest.Config{
				Host: tc.host,
			}

			name, err := getClusterInfo(config)

			if tc.expectedError {
				require.Error(t, err)
				if tc.errorSubstring != "" {
					require.Contains(t, err.Error(), tc.errorSubstring)
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expectedName, name)
			}
		})
	}
}

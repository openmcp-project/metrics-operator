package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	insight "github.com/openmcp-project/metrics-operator/api/v1alpha1"
	orc "github.com/openmcp-project/metrics-operator/internal/orchestrator"
)

// MockClient is a custom mock implementation of client.Client
type MockClient struct {
	GetFunc         func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
	SubResourceFunc func(subResource string) client.SubResourceClient
}

// ... (other methods remain the same)

func (m *MockClient) SubResource(subResource string) client.SubResourceClient {
	if m.SubResourceFunc != nil {
		return m.SubResourceFunc(subResource)
	}
	return nil
}

// MockSubResourceClient implements client.SubResourceClient
type MockSubResourceClient struct {
	CreateFunc func(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error
}

func (m *MockSubResourceClient) Get(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceGetOption) error {
	return nil
}

func (m *MockSubResourceClient) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return nil
}

func (m *MockSubResourceClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return nil
}

func (m *MockSubResourceClient) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, obj, subResource, opts...)
	}
	return nil
}

// Update the Get method to match the new interface
func (m *MockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return m.GetFunc(ctx, key, obj, opts...)
}

// Implement other methods of client.Client interface with empty implementations
func (m *MockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}
func (m *MockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return nil
}
func (m *MockClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return nil
}
func (m *MockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return nil
}
func (m *MockClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return nil
}
func (m *MockClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return nil
}
func (m *MockClient) Status() client.StatusWriter {
	return nil
}
func (m *MockClient) Scheme() *runtime.Scheme {
	return nil
}
func (m *MockClient) RESTMapper() meta.RESTMapper {
	return nil
}

func (m *MockClient) GroupVersionKindFor(_ runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}
func (m *MockClient) IsObjectNamespaced(_ runtime.Object) (bool, error) {
	return true, nil
}

func TestCreateExternalQueryConfig(t *testing.T) {
	tests := []struct {
		name            string
		racRef          *insight.RemoteClusterAccessRef
		mockGet         func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
		mockSubResource func(subResource string) client.SubResourceClient
		want            *orc.QueryConfig
		wantErr         bool
	}{
		{
			name: "Successfully create query config from KubeConfig",
			racRef: &insight.RemoteClusterAccessRef{
				Name:      "test-rca",
				Namespace: "default",
			},
			mockGet: func(_ context.Context, _ client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				switch obj := obj.(type) {
				case *insight.RemoteClusterAccess:
					*obj = insight.RemoteClusterAccess{
						Spec: insight.RemoteClusterAccessSpec{
							KubeConfigSecretRef: &insight.KubeConfigSecretRef{
								Name:      "test-secret",
								Namespace: "default",
								Key:       "kubeconfig",
							},
						},
					}
				case *corev1.Secret:
					*obj = corev1.Secret{
						Data: map[string][]byte{
							"kubeconfig": []byte(`
apiVersion: v1
clusters:
- cluster:
    server: https://example.com
  name: test-cluster
contexts:
- context:
    cluster: test-cluster
    user: test-user
  name: test-context
current-context: test-context
kind: Config
users:
- name: test-user
  user:
    token: test-token
`),
						},
					}
				}
				return nil
			},
			want: &orc.QueryConfig{
				ClusterName: ptr.To("test-cluster"),
			},
			wantErr: false,
		},
		// Add more test cases here
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockClient{
				GetFunc:         tt.mockGet,
				SubResourceFunc: tt.mockSubResource,
			}

			got, err := CreateExternalQueryConfig(context.Background(), tt.racRef, mockClient)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				require.Equal(t, tt.want.ClusterName, got.ClusterName)
				// Add more assertions based on your requirements
			}
		})
	}
}

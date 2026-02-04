package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	discoveryfake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
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

var _ client.Client = &MockClient{}

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
func (m *MockClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
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
							"kubeconfig": []byte(createDummyKubeconfigAsString()),
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

func TestCreateExternalQueryConfigSet(t *testing.T) {
	// Example test structure for when proper mocking is available:
	tests := []struct {
		name                   string
		fcaRef                 insight.FederateClusterAccessRef
		mockGet                func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
		fakeDynamicObjects     []runtime.Object
		fakeDiscoveryResources []*metav1.APIResourceList
		wantConfigCount        int
		wantErr                bool
		wantErrContains        string
	}{
		{
			name: "Successfully create query config set without selectors and a string type target",
			fcaRef: insight.FederateClusterAccessRef{
				Name:      "test-fca",
				Namespace: "default",
			},
			mockGet: func(_ context.Context, _ client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				switch obj := obj.(type) {
				case *insight.FederatedClusterAccess:
					*obj = insight.FederatedClusterAccess{
						Spec: insight.FederatedClusterAccessSpec{
							Target: insight.GroupVersionKind{
								Group:   "",
								Version: "v1",
								Kind:    "ConfigMap",
							},
							KubeConfigPath: "data.kubeconfig",
						},
					}
				}
				return nil
			},
			fakeDynamicObjects: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "default",
					},
					Data: map[string]string{
						"kubeconfig": createDummyKubeconfigAsString(),
					},
				},
			},
			fakeDiscoveryResources: []*metav1.APIResourceList{
				{
					GroupVersion: "v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "configmaps",
							Kind:       "ConfigMap",
							Namespaced: true,
						},
					},
				},
			},
			wantConfigCount: 1,
			wantErr:         false,
		},
		{
			name: "Successfully create query config set without selectors and a object type target",
			fcaRef: insight.FederateClusterAccessRef{
				Name:      "test-fca",
				Namespace: "default",
			},
			mockGet: func(_ context.Context, _ client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				switch obj := obj.(type) {
				case *insight.FederatedClusterAccess:
					*obj = insight.FederatedClusterAccess{
						Spec: insight.FederatedClusterAccessSpec{
							Target: insight.GroupVersionKind{
								Group:   "test",
								Version: "v1",
								Kind:    "DataObject",
							},
							KubeConfigPath: "data.kubeconfig",
						},
					}
				}
				return nil
			},
			fakeDynamicObjects: []runtime.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "test/v1",
						"kind":       "DataObject",
						"metadata": map[string]interface{}{
							"name":      "test",
							"namespace": "default",
						},
						"data": map[string]interface{}{
							"kubeconfig": createDummyKubeconfigAsObject(),
						},
					},
				},
			},
			fakeDiscoveryResources: []*metav1.APIResourceList{
				{
					GroupVersion: "test/v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "dataobjects",
							Kind:       "DataObject",
							Namespaced: true,
						},
					},
				},
			},
			wantConfigCount: 1,
			wantErr:         false,
		},
		{
			name: "Successfully create query config set without selectors and a secret reference type target (only name set)",
			fcaRef: insight.FederateClusterAccessRef{
				Name:      "test-fca",
				Namespace: "default",
			},
			mockGet: func(_ context.Context, _ client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				switch obj := obj.(type) {
				case *insight.FederatedClusterAccess:
					*obj = insight.FederatedClusterAccess{
						Spec: insight.FederatedClusterAccessSpec{
							Target: insight.GroupVersionKind{
								Group:   "test",
								Version: "v1",
								Kind:    "DataObject",
							},
							SecretRefPath: "data.kubeconfigRef",
						},
					}
				case *corev1.Secret:
					*obj = corev1.Secret{
						Data: map[string][]byte{
							"kubeconfig": []byte(createDummyKubeconfigAsString()),
						},
					}
				}
				return nil
			},
			fakeDynamicObjects: []runtime.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "test/v1",
						"kind":       "DataObject",
						"metadata": map[string]interface{}{
							"name":      "test",
							"namespace": "test-system",
						},
						"data": map[string]interface{}{
							"kubeconfigRef": map[string]interface{}{
								"name": "kube-secret",
							},
						},
					},
				},
			},
			fakeDiscoveryResources: []*metav1.APIResourceList{
				{
					GroupVersion: "test/v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "dataobjects",
							Kind:       "DataObject",
							Namespaced: true,
						},
					},
				},
			},
			wantConfigCount: 1,
			wantErr:         false,
		},
		{
			name: "Successfully create query config set without selectors and a secret reference type target (only name, namespace set)",
			fcaRef: insight.FederateClusterAccessRef{
				Name:      "test-fca",
				Namespace: "default",
			},
			mockGet: func(_ context.Context, _ client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				switch obj := obj.(type) {
				case *insight.FederatedClusterAccess:
					*obj = insight.FederatedClusterAccess{
						Spec: insight.FederatedClusterAccessSpec{
							Target: insight.GroupVersionKind{
								Group:   "test",
								Version: "v1",
								Kind:    "DataObject",
							},
							SecretRefPath: "data.kubeconfigRef",
						},
					}
				case *corev1.Secret:
					*obj = corev1.Secret{
						Data: map[string][]byte{
							"kubeconfig": []byte(createDummyKubeconfigAsString()),
						},
					}
				}
				return nil
			},
			fakeDynamicObjects: []runtime.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "test/v1",
						"kind":       "DataObject",
						"metadata": map[string]interface{}{
							"name":      "test",
							"namespace": "test-system",
						},
						"data": map[string]interface{}{
							"kubeconfigRef": map[string]interface{}{
								"name":      "kube-secret",
								"namespace": "custom-namespace",
							},
						},
					},
				},
			},
			fakeDiscoveryResources: []*metav1.APIResourceList{
				{
					GroupVersion: "test/v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "dataobjects",
							Kind:       "DataObject",
							Namespaced: true,
						},
					},
				},
			},
			wantConfigCount: 1,
			wantErr:         false,
		},
		{
			name: "Successfully create query config set without selectors and a secret reference type target (all set)",
			fcaRef: insight.FederateClusterAccessRef{
				Name:      "test-fca",
				Namespace: "default",
			},
			mockGet: func(_ context.Context, _ client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				switch obj := obj.(type) {
				case *insight.FederatedClusterAccess:
					*obj = insight.FederatedClusterAccess{
						Spec: insight.FederatedClusterAccessSpec{
							Target: insight.GroupVersionKind{
								Group:   "test",
								Version: "v1",
								Kind:    "DataObject",
							},
							SecretRefPath: "data.kubeconfigRef",
						},
					}
				case *corev1.Secret:
					*obj = corev1.Secret{
						Data: map[string][]byte{
							"theKubeconfig": []byte(createDummyKubeconfigAsString()),
						},
					}
				}
				return nil
			},
			fakeDynamicObjects: []runtime.Object{
				&unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "test/v1",
						"kind":       "DataObject",
						"metadata": map[string]interface{}{
							"name":      "test",
							"namespace": "test-system",
						},
						"data": map[string]interface{}{
							"kubeconfigRef": map[string]interface{}{
								"name":      "kube-secret",
								"namespace": "custom-namespace",
								"key":       "theKubeconfig",
							},
						},
					},
				},
			},
			fakeDiscoveryResources: []*metav1.APIResourceList{
				{
					GroupVersion: "test/v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "dataobjects",
							Kind:       "DataObject",
							Namespaced: true,
						},
					},
				},
			},
			wantConfigCount: 1,
			wantErr:         false,
		},
		{
			name: "Successfully create query config set with label selector",
			fcaRef: insight.FederateClusterAccessRef{
				Name:      "test-fca",
				Namespace: "default",
			},
			mockGet: func(_ context.Context, _ client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				switch obj := obj.(type) {
				case *insight.FederatedClusterAccess:
					*obj = insight.FederatedClusterAccess{
						Spec: insight.FederatedClusterAccessSpec{
							Target: insight.GroupVersionKind{
								Group:   "",
								Version: "v1",
								Kind:    "ConfigMap",
							},
							KubeConfigPath: "data.kubeconfig",
							LabelSelector:  "env=testing",
						},
					}
				}
				return nil
			},
			fakeDynamicObjects: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test1",
						Namespace: "default",
						Labels: map[string]string{
							"env": "testing",
						},
					},
					Data: map[string]string{
						"kubeconfig": createDummyKubeconfigAsString(),
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test2",
						Namespace: "default",
						Labels: map[string]string{
							"env": "testing",
						},
					},
					Data: map[string]string{
						"kubeconfig": createDummyKubeconfigAsString(),
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test3",
						Namespace: "default",
						Labels: map[string]string{
							"env": "productive",
						},
					},
					Data: map[string]string{
						"kubeconfig": createDummyKubeconfigAsString(),
					},
				},
			},
			fakeDiscoveryResources: []*metav1.APIResourceList{
				{
					GroupVersion: "v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "configmaps",
							Kind:       "ConfigMap",
							Namespaced: true,
						},
					},
				},
			},
			wantConfigCount: 2,
			wantErr:         false,
		},
		{
			name: "Successfully create query config set with field selector (fake since field selectors are applied server-side)",
			fcaRef: insight.FederateClusterAccessRef{
				Name:      "test-fca",
				Namespace: "default",
			},
			mockGet: func(_ context.Context, _ client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				switch obj := obj.(type) {
				case *insight.FederatedClusterAccess:
					*obj = insight.FederatedClusterAccess{
						Spec: insight.FederatedClusterAccessSpec{
							Target: insight.GroupVersionKind{
								Group:   "",
								Version: "v1",
								Kind:    "ConfigMap",
							},
							KubeConfigPath: "data.kubeconfig",
							FieldSelector:  "metadata.name=test1,metadata.name=test2",
						},
					}
				}
				return nil
			},
			fakeDynamicObjects: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test1",
						Namespace: "default",
					},
					Data: map[string]string{
						"kubeconfig": createDummyKubeconfigAsString(),
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test2",
						Namespace: "default",
					},
					Data: map[string]string{
						"kubeconfig": createDummyKubeconfigAsString(),
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test3",
						Namespace: "default",
					},
					Data: map[string]string{
						"kubeconfig": createDummyKubeconfigAsString(),
					},
				},
			},
			fakeDiscoveryResources: []*metav1.APIResourceList{
				{
					GroupVersion: "v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "configmaps",
							Kind:       "ConfigMap",
							Namespaced: true,
						},
					},
				},
			},
			// Attention: In real Kubernetes client-go, field selectors are not applied client-side but server-side.
			// This test assumes that the fake client applies them, which does not reflect real behavior.
			// Therefore, all three objects are returned here.
			// This test just executes the code path and does not validate field selector functionality.
			wantConfigCount: 3,
			wantErr:         false,
		},
		{
			name: "Successfully create query config set with label selector and namespace",
			fcaRef: insight.FederateClusterAccessRef{
				Name:      "test-fca",
				Namespace: "default",
			},
			mockGet: func(_ context.Context, _ client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				switch obj := obj.(type) {
				case *insight.FederatedClusterAccess:
					*obj = insight.FederatedClusterAccess{
						Spec: insight.FederatedClusterAccessSpec{
							Target: insight.GroupVersionKind{
								Group:   "",
								Version: "v1",
								Kind:    "ConfigMap",
							},
							KubeConfigPath: "data.kubeconfig",
							LabelSelector:  "env=testing",
							Namespace:      "special",
						},
					}
				}
				return nil
			},
			fakeDynamicObjects: []runtime.Object{
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test1",
						Namespace: "default",
						Labels: map[string]string{
							"env": "testing",
						},
					},
					Data: map[string]string{
						"kubeconfig": createDummyKubeconfigAsString(),
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test2",
						Namespace: "special",
						Labels: map[string]string{
							"env": "testing",
						},
					},
					Data: map[string]string{
						"kubeconfig": createDummyKubeconfigAsString(),
					},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test3",
						Namespace: "special",
						Labels: map[string]string{
							"env": "productive",
						},
					},
					Data: map[string]string{
						"kubeconfig": createDummyKubeconfigAsString(),
					},
				},
			},
			fakeDiscoveryResources: []*metav1.APIResourceList{
				{
					GroupVersion: "v1",
					APIResources: []metav1.APIResource{
						{
							Name:       "configmaps",
							Kind:       "ConfigMap",
							Namespaced: true,
						},
					},
				},
			},
			wantConfigCount: 1,
			wantErr:         false,
		},
	}

	dummyRestConfig := &rest.Config{}
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockClient{
				GetFunc: tt.mockGet,
			}

			// Create a fake discovery client
			fakeDiscovery := &discoveryfake.FakeDiscovery{
				Fake: &clienttesting.Fake{},
			}
			fakeDiscovery.Resources = tt.fakeDiscoveryResources

			// Create a scheme and register the types we'll be using
			fakeDynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, tt.fakeDynamicObjects...)

			// Create option functions to inject the fake clients
			getFakeDiscoveryClientFunc := func(restConfig *rest.Config) (discovery.DiscoveryInterface, error) {
				return fakeDiscovery, nil
			}

			getFakeDynamicClientFunc := func(restConfig *rest.Config) (dynamic.Interface, error) {
				return fakeDynamicClient, nil
			}

			opts := CreateExternalQueryConfigSetOptions{
				GetDynamicClient:   getFakeDynamicClientFunc,
				GetDiscoveryClient: getFakeDiscoveryClientFunc,
			}

			// Use functional options pattern
			got, err := CreateExternalQueryConfigSet(
				context.Background(),
				tt.fcaRef,
				mockClient,
				dummyRestConfig,
				opts,
			)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrContains != "" {
					require.Contains(t, err.Error(), tt.wantErrContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, got)
				require.Equal(t, tt.wantConfigCount, len(got))
			}
		})
	}
}

func createDummyKubeconfigAsString() string {
	return `
apiVersion: v1
kind: Config
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
users:
- name: test-user
  user:
    token: test-token
`
}

func createDummyKubeconfigAsObject() map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Config",
		"clusters": []interface{}{
			map[string]interface{}{
				"cluster": map[string]interface{}{
					"server": "https://example.com",
				},
				"name": "test-cluster",
			},
		},
		"contexts": []interface{}{
			map[string]interface{}{
				"context": map[string]interface{}{
					"cluster": "test-cluster",
					"user":    "test-user",
				},
				"name": "test-context",
			},
		},
		"current-context": "test-context",
		"users": []interface{}{
			map[string]interface{}{
				"name": "test-user",
				"user": map[string]interface{}{
					"token": "test-token",
				},
			},
		},
	}
}

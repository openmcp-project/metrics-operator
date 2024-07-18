package config

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// customFakeClient is a custom implementation of client.Client
type customFakeClient struct {
	client.Client
}

// SubResource returns a custom SubResourceClient
func (c *customFakeClient) SubResource(subResourceName string) client.SubResourceClient {
	return &fakeSubResourceClient{
		createFn: func(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
			tr, ok := subResource.(*authenticationv1.TokenRequest)
			if !ok {
				return nil
			}
			tr.Status.Token = uuid.New().String()
			tr.Status.ExpirationTimestamp = metav1.NewTime(time.Now().Add(2 * time.Hour))
			return nil
		},
	}
}

// fakeSubResourceClient is a mock implementation of client.SubResourceClient
type fakeSubResourceClient struct {
	createFn func(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error
}

func (f *fakeSubResourceClient) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return f.createFn(ctx, obj, subResource, opts...)
}

func (f *fakeSubResourceClient) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return nil
}

func (f *fakeSubResourceClient) Get(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceGetOption) error {
	return nil
}

func (f *fakeSubResourceClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return nil
}

func TestGetTokenManager(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = authenticationv1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	customClient := &customFakeClient{Client: fakeClient}

	tm1, err := GetTokenManager(customClient)
	require.NoError(t, err)
	require.NotNil(t, tm1)

	tm2, err := GetTokenManager(customClient)
	require.NoError(t, err)
	require.NotNil(t, tm2)

	assert.Equal(t, tm1, tm2, "GetTokenManager should return the same instance")
}

func TestGetToken_Cache_Valid(t *testing.T) {
	// Setup
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = authenticationv1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	customClient := &customFakeClient{Client: fakeClient}

	tm, err := newTokenManager(customClient)
	require.NoError(t, err)

	// Test getting a new token
	tk, err := tm.GetToken("default", "test-sa", "test-audience")
	require.NoError(t, err)
	assert.NotEmpty(t, tk)

	// Test getting the same token from cache
	ct, err := tm.GetToken("default", "test-sa", "test-audience")
	require.NoError(t, err)
	assert.Equal(t, tk, ct)

}

func TestGetToken_Cache_Expired(t *testing.T) {
	// Setup
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = authenticationv1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	cli := &customFakeClient{Client: fakeClient}

	tm, err := newTokenManager(cli)
	require.NoError(t, err)

	// Test getting a new token
	tk, err := tm.GetToken("default", "test-sa", "test-audience")
	require.NoError(t, err)
	assert.NotEmpty(t, tk)

	// Test token refresh
	tm.refreshBuffer = 50 * time.Hour // Force refresh
	rt, err := tm.GetToken("default", "test-sa", "test-audience")
	require.NoError(t, err)
	assert.NotEqual(t, tk, rt)
}

//func TestTokenManager(t *testing.T) {
//
//	config, errrc := clientcmd.BuildConfigFromFlags("", "/Users/I073426/Desktop/metric-demo/dev-core.yaml")
//	if errrc != nil {
//		fmt.Println(errrc)
//	}
//
//	clientset, _ := kubernetes.NewForConfig(config)
//
//	tokenManager, err := GetTokenManager(clientset)
//	if err != nil {
//		panic(err)
//	}
//
//	token, err := tokenManager.GetToken("cola-system", "mcp-operator", "crate")
//	if err != nil {
//		// Handle error
//	}
//
//	tokenManager2, err := GetTokenManager(clientset)
//	if err != nil {
//		panic(err)
//	}
//
//	token2, err := tokenManager2.GetToken("cola-system", "mcp-operator", "crate")
//	if err != nil {
//		// Handle error
//	}
//
//	assert.Equal(t, token, token2)
//
//	fmt.Println(token)
//
//}

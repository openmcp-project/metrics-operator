package config

import (
	"context"
	"fmt"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	instance *TokenManager
	once     sync.Once
)

var (
	marginFromExpirationTime = 5 * time.Minute // 5 minutes before actual expiration time
	tokenTimeToLife          = 7200            // 2 hours
	cacheSize                = 10              // one token per cluster access, so 10 clusters max (assuming, 1 service account with roles per cluster)
)

// TokenManager is a struct that manages the token for a service account.
// It caches the token and refreshes it when it is about to expire.
// It is a singleton.
type TokenManager struct {
	client client.Client
	cache  *lru.Cache[string, cachedToken]

	// refreshBuffer is the time before the actual expiration time to refresh the token
	refreshBuffer time.Duration
}

type cachedToken struct {
	token      string
	expiration time.Time
}

type tokenKey struct {
	serviceAccount   string
	serviceNamespace string
	audience         string
}

// GetTokenManager returns the singleton instance of TokenManager.
func GetTokenManager(cli client.Client) (*TokenManager, error) {
	var err error
	once.Do(func() {
		instance, err = newTokenManager(cli)
	})
	if err != nil {
		return nil, err
	}
	return instance, nil
}

func newTokenManager(client client.Client) (*TokenManager, error) {
	cache, err := lru.New[string, cachedToken](cacheSize) // We only need one token per cluster
	if err != nil {
		return nil, fmt.Errorf("failed to create lru cache: %w", err)
	}

	return &TokenManager{
		client:        client,
		cache:         cache,
		refreshBuffer: marginFromExpirationTime, // expire 5 minutes before actual expiration to be safe, in k8s min is 10 minutes
	}, nil
}

func (tk *tokenKey) getKey() string {
	return fmt.Sprintf("%s-%s-%s", tk.serviceAccount, tk.serviceNamespace, tk.audience)
}

// GetToken returns a token for specified, service account and audience (clientId in OpenID).
func (tm *TokenManager) GetToken(ctx context.Context, namespace, serviceAccount, audience string) (string, error) {
	uniqueTokenKey := tokenKey{
		serviceAccount:   serviceAccount,
		serviceNamespace: namespace,
		audience:         audience,
	}
	key := uniqueTokenKey.getKey()

	if cachedToken, ok := tm.cache.Get(key); ok {
		if time.Now().Add(tm.refreshBuffer).Before(cachedToken.expiration) {
			return cachedToken.token, nil
		}
	}

	return tm.refreshToken(ctx, uniqueTokenKey)
}

func (tm *TokenManager) refreshToken(ctx context.Context, utk tokenKey) (string, error) {
	tr := &authenticationv1.TokenRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: utk.serviceNamespace,
			Name:      utk.serviceAccount,
		},
		Spec: authenticationv1.TokenRequestSpec{
			Audiences:         []string{utk.audience},
			ExpirationSeconds: ptr.To(int64(tokenTimeToLife)), // 2 hours
		},
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utk.serviceAccount,
			Namespace: utk.serviceNamespace,
		},
	}

	err := tm.client.SubResource("token").Create(ctx, sa, tr, &client.SubResourceCreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create token from service account '%s/%s': %w", utk.serviceNamespace, utk.serviceAccount, err)
	}

	newToken := cachedToken{
		token:      tr.Status.Token,
		expiration: tr.Status.ExpirationTimestamp.Time,
	}
	// no need to check for eviction, we only cache one token per unique key
	// if it is still there by the time we add it, it will be evicted and replaced with the new one
	tm.cache.Add(utk.getKey(), newToken)

	return newToken.token, nil
}

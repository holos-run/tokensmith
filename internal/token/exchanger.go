package token

import (
	"context"
	"fmt"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ExchangeConfig holds configuration for token exchange.
type ExchangeConfig struct {
	// Audiences is the list of audiences for the requested token.
	// Defaults to ["https://kubernetes.default.svc"]
	Audiences []string

	// ExpirationSeconds is the requested token expiration in seconds.
	// If not specified, defaults to 1 hour (3600 seconds).
	ExpirationSeconds *int64
}

// Exchanger exchanges tokens using the Kubernetes TokenRequest API.
type Exchanger struct {
	client kubernetes.Interface
	config ExchangeConfig
}

// NewExchanger creates a new token exchanger.
func NewExchanger(client kubernetes.Interface, config ExchangeConfig) *Exchanger {
	// Set default audiences if not provided
	if len(config.Audiences) == 0 {
		config.Audiences = []string{"https://kubernetes.default.svc"}
	}

	// Set default expiration if not provided (1 hour)
	if config.ExpirationSeconds == nil {
		defaultExpiration := int64(3600)
		config.ExpirationSeconds = &defaultExpiration
	}

	return &Exchanger{
		client: client,
		config: config,
	}
}

// Exchange exchanges a validated service account identity for a new token
// in the management cluster.
//
// The service account with the same namespace and name must exist in the
// management cluster. RBAC policies control what the service account can access.
func (e *Exchanger) Exchange(ctx context.Context, identity *ServiceAccountIdentity) (string, error) {
	// Verify service account exists in management cluster
	sa, err := e.client.CoreV1().ServiceAccounts(identity.Namespace).Get(ctx, identity.Name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("service account %s/%s not found in management cluster: %w",
			identity.Namespace, identity.Name, err)
	}

	// Create TokenRequest
	tokenRequest := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences:         e.config.Audiences,
			ExpirationSeconds: e.config.ExpirationSeconds,
		},
	}

	// Call Kubernetes API to create token
	result, err := e.client.CoreV1().ServiceAccounts(identity.Namespace).CreateToken(
		ctx,
		identity.Name,
		tokenRequest,
		metav1.CreateOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("failed to create token for service account %s/%s: %w",
			identity.Namespace, identity.Name, err)
	}

	// Extract token from result
	token := result.Status.Token
	if token == "" {
		return "", fmt.Errorf("received empty token from TokenRequest API")
	}

	// Log token metadata (for debugging, not the actual token)
	_ = sa.UID // Service account UID for audit logging

	return token, nil
}

// ExchangeWithMetadata exchanges a token and returns both the token and metadata.
type TokenMetadata struct {
	Token             string
	Namespace         string
	ServiceAccount    string
	ExpirationTime    time.Time
	ServiceAccountUID string
}

// ExchangeWithMetadata exchanges a token and returns detailed metadata.
func (e *Exchanger) ExchangeWithMetadata(ctx context.Context, identity *ServiceAccountIdentity) (*TokenMetadata, error) {
	// Verify service account exists in management cluster
	sa, err := e.client.CoreV1().ServiceAccounts(identity.Namespace).Get(ctx, identity.Name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("service account %s/%s not found in management cluster: %w",
			identity.Namespace, identity.Name, err)
	}

	// Create TokenRequest
	tokenRequest := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences:         e.config.Audiences,
			ExpirationSeconds: e.config.ExpirationSeconds,
		},
	}

	// Call Kubernetes API to create token
	result, err := e.client.CoreV1().ServiceAccounts(identity.Namespace).CreateToken(
		ctx,
		identity.Name,
		tokenRequest,
		metav1.CreateOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create token for service account %s/%s: %w",
			identity.Namespace, identity.Name, err)
	}

	// Extract token from result
	token := result.Status.Token
	if token == "" {
		return nil, fmt.Errorf("received empty token from TokenRequest API")
	}

	return &TokenMetadata{
		Token:             token,
		Namespace:         identity.Namespace,
		ServiceAccount:    identity.Name,
		ExpirationTime:    result.Status.ExpirationTimestamp.Time,
		ServiceAccountUID: string(sa.UID),
	}, nil
}

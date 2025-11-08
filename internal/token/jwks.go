package token

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/holos-run/tokensmith/internal/config"
)

// JWKSValidator validates JWT tokens using JWKS.
type JWKSValidator struct {
	config *config.ClustersConfig
	client *http.Client
	cache  map[string]*cachedJWKS
	mu     sync.RWMutex
}

// cachedJWKS holds a cached JWKS with its fetch time.
type cachedJWKS struct {
	jwks      *jose.JSONWebKeySet
	fetchedAt time.Time
}

// NewJWKSValidator creates a new JWKS validator.
func NewJWKSValidator(cfg *config.ClustersConfig) *JWKSValidator {
	return &JWKSValidator{
		config: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		cache: make(map[string]*cachedJWKS),
	}
}

// Validate validates a JWT token and returns the service account identity.
func (v *JWKSValidator) Validate(ctx context.Context, tokenString string) (*ServiceAccountIdentity, error) {
	// Parse the token without verification first to extract claims
	tok, err := jwt.ParseSigned(tokenString, []jose.SignatureAlgorithm{
		jose.RS256,
		jose.RS384,
		jose.RS512,
		jose.ES256,
		jose.ES384,
		jose.ES512,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	// Extract claims to get the issuer
	var claims jwt.Claims
	var k8sClaims map[string]interface{}
	if err := tok.UnsafeClaimsWithoutVerification(&claims, &k8sClaims); err != nil {
		return nil, fmt.Errorf("failed to extract claims: %w", err)
	}

	// Find the cluster configuration by issuer
	clusterConfig := v.config.FindByIssuer(claims.Issuer)
	if clusterConfig == nil {
		return nil, fmt.Errorf("unknown issuer: %s", claims.Issuer)
	}

	// Get the JWKS for this cluster
	jwks, err := v.getJWKS(ctx, clusterConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get JWKS: %w", err)
	}

	// Verify the token signature and validate claims
	if err := tok.Claims(jwks, &claims, &k8sClaims); err != nil {
		return nil, fmt.Errorf("failed to verify token: %w", err)
	}

	// Validate standard JWT claims
	expected := jwt.Expected{
		Time: time.Now(),
	}
	if err := claims.Validate(expected); err != nil {
		return nil, fmt.Errorf("invalid claims: %w", err)
	}

	// Extract Kubernetes service account identity from claims
	identity, err := extractServiceAccountIdentity(&claims, k8sClaims)
	if err != nil {
		return nil, fmt.Errorf("failed to extract service account identity: %w", err)
	}

	return identity, nil
}

// getJWKS returns the JWKS for a cluster, using cache or fetching as needed.
func (v *JWKSValidator) getJWKS(ctx context.Context, cluster *config.ClusterConfig) (*jose.JSONWebKeySet, error) {
	// If inline JWKS data is provided, use it directly
	if cluster.JWKSData != nil {
		return cluster.JWKSData, nil
	}

	// Check cache first
	v.mu.RLock()
	cached, ok := v.cache[cluster.JWKSURI]
	v.mu.RUnlock()

	// Use cached JWKS if it's fresh (less than 1 hour old)
	if ok && time.Since(cached.fetchedAt) < time.Hour {
		return cached.jwks, nil
	}

	// Fetch JWKS from URI
	jwks, err := v.fetchJWKS(ctx, cluster.JWKSURI)
	if err != nil {
		return nil, err
	}

	// Update cache
	v.mu.Lock()
	v.cache[cluster.JWKSURI] = &cachedJWKS{
		jwks:      jwks,
		fetchedAt: time.Now(),
	}
	v.mu.Unlock()

	return jwks, nil
}

// fetchJWKS fetches a JWKS from a URI.
func (v *JWKSValidator) fetchJWKS(ctx context.Context, uri string) (*jose.JSONWebKeySet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch JWKS: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var jwks jose.JSONWebKeySet
	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, fmt.Errorf("failed to parse JWKS: %w", err)
	}

	return &jwks, nil
}

// extractServiceAccountIdentity extracts the service account identity from JWT claims.
func extractServiceAccountIdentity(claims *jwt.Claims, k8sClaims map[string]interface{}) (*ServiceAccountIdentity, error) {
	// Extract namespace from kubernetes.io/serviceaccount/namespace claim
	namespace, ok := k8sClaims["kubernetes.io/serviceaccount/namespace"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid kubernetes.io/serviceaccount/namespace claim")
	}

	// Extract service account name from kubernetes.io/serviceaccount/service-account.name claim
	name, ok := k8sClaims["kubernetes.io/serviceaccount/service-account.name"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid kubernetes.io/serviceaccount/service-account.name claim")
	}

	// Extract UID from kubernetes.io/serviceaccount/service-account.uid claim
	uid, ok := k8sClaims["kubernetes.io/serviceaccount/service-account.uid"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid kubernetes.io/serviceaccount/service-account.uid claim")
	}

	// Construct username in the standard Kubernetes format
	username := fmt.Sprintf("system:serviceaccount:%s:%s", namespace, name)

	return &ServiceAccountIdentity{
		Namespace: namespace,
		Name:      name,
		UID:       uid,
		Username:  username,
	}, nil
}

package token

import (
	"context"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"github.com/holos-run/tokensmith/internal/config"
	"github.com/holos-run/tokensmith/internal/testutil"
)

// Helper function to convert RSA public key to JWKS format
func rsaPublicKeyToJWK(key *rsa.PublicKey, kid string) jose.JSONWebKey {
	return jose.JSONWebKey{
		Key:       key,
		KeyID:     kid,
		Algorithm: string(jose.RS256),
		Use:       "sig",
	}
}

// Helper function to create a JWKS with a public key
func createJWKS(publicKey *rsa.PublicKey, kid string) *jose.JSONWebKeySet {
	jwk := rsaPublicKeyToJWK(publicKey, kid)
	return &jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{jwk},
	}
}

func TestJWKSValidator_Validate(t *testing.T) {
	ctx := context.Background()

	// Create test issuers
	issuer1 := "https://cluster1.example.com"
	issuer2 := "https://cluster2.example.com"

	// Create JWT signers for two clusters
	signer1, err := testutil.NewJWTSigner(issuer1)
	if err != nil {
		t.Fatalf("Failed to create signer1: %v", err)
	}

	signer2, err := testutil.NewJWTSigner(issuer2)
	if err != nil {
		t.Fatalf("Failed to create signer2: %v", err)
	}

	// Create JWKS for each cluster
	jwks1 := createJWKS(signer1.PublicKey(), signer1.KeyID())
	jwks2 := createJWKS(signer2.PublicKey(), signer2.KeyID())

	// Create clusters configuration
	cfg := &config.ClustersConfig{
		Clusters: []config.ClusterConfig{
			{
				Name:     "cluster1",
				Issuer:   issuer1,
				JWKSData: jwks1,
			},
			{
				Name:     "cluster2",
				Issuer:   issuer2,
				JWKSData: jwks2,
			},
		},
	}

	// Create validator
	validator := NewJWKSValidator(cfg)

	t.Run("validate token from cluster1", func(t *testing.T) {
		// Generate a valid token from cluster1 using flat claims
		token, err := signer1.GenerateTokenFlatClaims(
			"default",
			"my-service-account",
			uuid.New().String(),
			[]string{"https://kubernetes.default.svc"},
			time.Now().Add(1*time.Hour),
		)
		if err != nil {
			t.Fatalf("Failed to generate token: %v", err)
		}

		// Validate the token
		identity, err := validator.Validate(ctx, token)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}

		// Check identity
		if identity.Namespace != "default" {
			t.Errorf("Expected namespace 'default', got %q", identity.Namespace)
		}
		if identity.Name != "my-service-account" {
			t.Errorf("Expected name 'my-service-account', got %q", identity.Name)
		}
		if identity.Username != "system:serviceaccount:default:my-service-account" {
			t.Errorf("Expected username 'system:serviceaccount:default:my-service-account', got %q", identity.Username)
		}
	})

	t.Run("validate token from cluster2", func(t *testing.T) {
		// Generate a valid token from cluster2 using flat claims
		token, err := signer2.GenerateTokenFlatClaims(
			"kube-system",
			"admin-sa",
			uuid.New().String(),
			[]string{"https://kubernetes.default.svc"},
			time.Now().Add(1*time.Hour),
		)
		if err != nil {
			t.Fatalf("Failed to generate token: %v", err)
		}

		// Validate the token
		identity, err := validator.Validate(ctx, token)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}

		// Check identity
		if identity.Namespace != "kube-system" {
			t.Errorf("Expected namespace 'kube-system', got %q", identity.Namespace)
		}
		if identity.Name != "admin-sa" {
			t.Errorf("Expected name 'admin-sa', got %q", identity.Name)
		}
	})

	t.Run("reject token from unknown issuer", func(t *testing.T) {
		// Create a signer with unknown issuer
		unknownSigner, err := testutil.NewJWTSigner("https://unknown.example.com")
		if err != nil {
			t.Fatalf("Failed to create unknown signer: %v", err)
		}

		token, err := unknownSigner.GenerateTokenFlatClaims(
			"default",
			"test-sa",
			uuid.New().String(),
			[]string{"https://kubernetes.default.svc"},
			time.Now().Add(1*time.Hour),
		)
		if err != nil {
			t.Fatalf("Failed to generate token: %v", err)
		}

		// Validation should fail
		_, err = validator.Validate(ctx, token)
		if err == nil {
			t.Fatal("Expected validation to fail for unknown issuer")
		}
		if err.Error() != "unknown issuer: https://unknown.example.com" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("reject expired token", func(t *testing.T) {
		// Generate an expired token using flat claims
		token, err := signer1.GenerateTokenFlatClaims(
			"default",
			"test-sa",
			uuid.New().String(),
			[]string{"https://kubernetes.default.svc"},
			time.Now().Add(-1*time.Hour), // Expired 1 hour ago
		)
		if err != nil {
			t.Fatalf("Failed to generate token: %v", err)
		}

		// Validation should fail
		_, err = validator.Validate(ctx, token)
		if err == nil {
			t.Fatal("Expected validation to fail for expired token")
		}
	})

	t.Run("reject token with invalid signature", func(t *testing.T) {
		// Create a different signer (different key)
		wrongSigner, err := testutil.NewJWTSigner(issuer1) // Same issuer but different key
		if err != nil {
			t.Fatalf("Failed to create wrong signer: %v", err)
		}

		token, err := wrongSigner.GenerateTokenFlatClaims(
			"default",
			"test-sa",
			uuid.New().String(),
			[]string{"https://kubernetes.default.svc"},
			time.Now().Add(1*time.Hour),
		)
		if err != nil {
			t.Fatalf("Failed to generate token: %v", err)
		}

		// Validation should fail due to signature mismatch
		_, err = validator.Validate(ctx, token)
		if err == nil {
			t.Fatal("Expected validation to fail for invalid signature")
		}
	})

	t.Run("reject malformed token", func(t *testing.T) {
		// Validation should fail for malformed token
		_, err := validator.Validate(ctx, "not.a.valid.jwt")
		if err == nil {
			t.Fatal("Expected validation to fail for malformed token")
		}
	})
}

func TestJWKSValidator_MultipleKeys(t *testing.T) {
	ctx := context.Background()
	issuer := "https://cluster.example.com"

	// Create two different signers (simulating key rotation)
	signer1, err := testutil.NewJWTSigner(issuer)
	if err != nil {
		t.Fatalf("Failed to create signer1: %v", err)
	}

	signer2, err := testutil.NewJWTSigner(issuer)
	if err != nil {
		t.Fatalf("Failed to create signer2: %v", err)
	}

	// Create JWKS with both keys (simulating key rotation scenario)
	jwk1 := rsaPublicKeyToJWK(signer1.PublicKey(), signer1.KeyID())
	jwk2 := rsaPublicKeyToJWK(signer2.PublicKey(), signer2.KeyID())
	jwks := &jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{jwk1, jwk2},
	}

	// Create configuration
	cfg := &config.ClustersConfig{
		Clusters: []config.ClusterConfig{
			{
				Name:     "cluster",
				Issuer:   issuer,
				JWKSData: jwks,
			},
		},
	}

	validator := NewJWKSValidator(cfg)

	t.Run("validate token signed with old key", func(t *testing.T) {
		token, err := signer1.GenerateTokenFlatClaims(
			"default",
			"test-sa",
			uuid.New().String(),
			[]string{"https://kubernetes.default.svc"},
			time.Now().Add(1*time.Hour),
		)
		if err != nil {
			t.Fatalf("Failed to generate token: %v", err)
		}

		identity, err := validator.Validate(ctx, token)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}

		if identity.Namespace != "default" {
			t.Errorf("Expected namespace 'default', got %q", identity.Namespace)
		}
	})

	t.Run("validate token signed with new key", func(t *testing.T) {
		token, err := signer2.GenerateTokenFlatClaims(
			"default",
			"test-sa",
			uuid.New().String(),
			[]string{"https://kubernetes.default.svc"},
			time.Now().Add(1*time.Hour),
		)
		if err != nil {
			t.Fatalf("Failed to generate token: %v", err)
		}

		identity, err := validator.Validate(ctx, token)
		if err != nil {
			t.Fatalf("Validation failed: %v", err)
		}

		if identity.Namespace != "default" {
			t.Errorf("Expected namespace 'default', got %q", identity.Namespace)
		}
	})
}

func TestExtractServiceAccountIdentity(t *testing.T) {
	tests := []struct {
		name        string
		k8sClaims   map[string]interface{}
		wantErr     bool
		wantNS      string
		wantName    string
		wantUID     string
		wantUser    string
	}{
		{
			name: "valid claims",
			k8sClaims: map[string]interface{}{
				"kubernetes.io/serviceaccount/namespace":              "default",
				"kubernetes.io/serviceaccount/service-account.name":   "my-sa",
				"kubernetes.io/serviceaccount/service-account.uid":    "123e4567-e89b-12d3-a456-426614174000",
			},
			wantErr:  false,
			wantNS:   "default",
			wantName: "my-sa",
			wantUID:  "123e4567-e89b-12d3-a456-426614174000",
			wantUser: "system:serviceaccount:default:my-sa",
		},
		{
			name: "missing namespace claim",
			k8sClaims: map[string]interface{}{
				"kubernetes.io/serviceaccount/service-account.name": "my-sa",
				"kubernetes.io/serviceaccount/service-account.uid":  "123",
			},
			wantErr: true,
		},
		{
			name: "missing name claim",
			k8sClaims: map[string]interface{}{
				"kubernetes.io/serviceaccount/namespace":           "default",
				"kubernetes.io/serviceaccount/service-account.uid": "123",
			},
			wantErr: true,
		},
		{
			name: "missing uid claim",
			k8sClaims: map[string]interface{}{
				"kubernetes.io/serviceaccount/namespace":            "default",
				"kubernetes.io/serviceaccount/service-account.name": "my-sa",
			},
			wantErr: true,
		},
		{
			name: "invalid type for namespace",
			k8sClaims: map[string]interface{}{
				"kubernetes.io/serviceaccount/namespace":            123, // Should be string
				"kubernetes.io/serviceaccount/service-account.name": "my-sa",
				"kubernetes.io/serviceaccount/service-account.uid":  "123",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We need to pass a jwt.Claims, but it's not used in extractServiceAccountIdentity
			// so we can pass nil
			identity, err := extractServiceAccountIdentity(nil, tt.k8sClaims)

			if tt.wantErr {
				if err == nil {
					t.Fatal("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if identity.Namespace != tt.wantNS {
				t.Errorf("Expected namespace %q, got %q", tt.wantNS, identity.Namespace)
			}
			if identity.Name != tt.wantName {
				t.Errorf("Expected name %q, got %q", tt.wantName, identity.Name)
			}
			if identity.UID != tt.wantUID {
				t.Errorf("Expected UID %q, got %q", tt.wantUID, identity.UID)
			}
			if identity.Username != tt.wantUser {
				t.Errorf("Expected username %q, got %q", tt.wantUser, identity.Username)
			}
		})
	}
}

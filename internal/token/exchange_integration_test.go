// exchange_integration_test.go tests the complete token exchange flow using
// fake Kubernetes clients and actual JWT token generation.
//
// These tests generate real Kubernetes service account JWTs and verify that:
//
// 1. Token validation correctly parses service account identity from JWT claims
// 2. Token exchange creates valid JWTs for the correct service account
// 3. JWT claims are preserved (namespace, name, audience, expiration)
// 4. Service account UIDs differ between workload and management clusters
// 5. Errors are handled appropriately (SA not found, validation fails)
//
// The approach mirrors Kubernetes upstream testing patterns for service account tokens.
//
// Run with: go test -v ./internal/token/

package token

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/holos-run/tokensmith/internal/testutil"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// UUID validation regex
var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// TestTokenExchangeFlow tests the happy path of token validation and exchange
func TestTokenExchangeFlow(t *testing.T) {
	ctx := context.Background()

	// Setup JWT signers for both clusters
	// Both use the same issuer - tokens are for the same logical cluster from the API perspective
	issuer := "https://kubernetes.default.svc.cluster.local"

	workloadSigner, err := testutil.NewJWTSigner(issuer)
	if err != nil {
		t.Fatalf("Failed to create workload JWT signer: %v", err)
	}

	managementSigner, err := testutil.NewJWTSigner(issuer)
	if err != nil {
		t.Fatalf("Failed to create management JWT signer: %v", err)
	}

	// Test data
	namespace := "default"
	saName := "default"
	workloadUID := "72b0e9c5-c44a-4de0-ae59-9b400f1221e0"
	managementUID := "a1b2c3d4-e5f6-4789-a0b1-c2d3e4f5a6b7"
	audiences := []string{"https://kubernetes.default.svc"}
	expiration := time.Now().Add(1 * time.Hour)

	// Generate workload cluster token
	workloadToken, err := workloadSigner.GenerateToken(namespace, saName, workloadUID, audiences, expiration)
	if err != nil {
		t.Fatalf("Failed to generate workload token: %v", err)
	}

	// Setup fake workload client with TokenReview reactor
	workloadClient := fake.NewSimpleClientset()
	workloadClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		createAction := action.(k8stesting.CreateAction)
		tr := createAction.GetObject().(*authenticationv1.TokenReview)

		// Mock the response
		tr.Status = authenticationv1.TokenReviewStatus{
			Authenticated: true,
			User: authenticationv1.UserInfo{
				Username: "system:serviceaccount:" + namespace + ":" + saName,
				UID:      workloadUID,
			},
		}
		return true, tr, nil
	})

	// Setup fake management client
	managementClient := fake.NewSimpleClientset()

	// Add service account to management cluster
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: namespace,
			UID:       types.UID(managementUID),
		},
	}
	_, err = managementClient.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create service account in fake client: %v", err)
	}

	// Add reactor to mock TokenRequest
	managementClient.PrependReactor("create", "serviceaccounts", func(action k8stesting.Action) (bool, runtime.Object, error) {
		// Check if this is a token request (subresource)
		if action.GetSubresource() == "token" {
			createAction := action.(k8stesting.CreateAction)
			tokenReq := createAction.GetObject().(*authenticationv1.TokenRequest)

			// Generate actual JWT token for management cluster
			jwtToken, err := managementSigner.GenerateToken(
				namespace,
				saName,
				managementUID,
				tokenReq.Spec.Audiences,
				expiration,
			)
			if err != nil {
				return true, nil, err
			}

			tokenReq.Status = authenticationv1.TokenRequestStatus{
				Token:               jwtToken,
				ExpirationTimestamp: metav1.Time{Time: expiration},
			}
			return true, tokenReq, nil
		}
		return false, nil, nil
	})

	// Create validator and exchanger
	validator := NewValidator(workloadClient)

	expirationSeconds := int64(3600)
	exchanger := NewExchanger(managementClient, ExchangeConfig{
		Audiences:         audiences,
		ExpirationSeconds: &expirationSeconds,
	})

	// Execute validation
	identity, err := validator.Validate(ctx, workloadToken)
	if err != nil {
		t.Fatalf("Token validation failed: %v", err)
	}

	// Verify identity
	if identity.Namespace != namespace {
		t.Errorf("Namespace mismatch: got %q, want %q", identity.Namespace, namespace)
	}
	if identity.Name != saName {
		t.Errorf("Service account name mismatch: got %q, want %q", identity.Name, saName)
	}

	// Execute exchange
	metadata, err := exchanger.ExchangeWithMetadata(ctx, identity)
	if err != nil {
		t.Fatalf("Token exchange failed: %v", err)
	}

	// Parse both tokens
	workloadClaims, err := workloadSigner.ParseToken(workloadToken)
	if err != nil {
		t.Fatalf("Failed to parse workload token: %v", err)
	}

	managementClaims, err := managementSigner.ParseToken(metadata.Token)
	if err != nil {
		t.Fatalf("Failed to parse management token: %v", err)
	}

	// ============================================================================
	// VERIFY CLAIMS THAT MUST BE IDENTICAL
	// ============================================================================

	// Subject (sub) - MUST be identical
	if workloadClaims.Subject != managementClaims.Subject {
		t.Errorf("Subject mismatch: %s != %s",
			workloadClaims.Subject, managementClaims.Subject)
	}

	// Audience (aud) - MUST be identical
	if len(workloadClaims.Audience) != len(managementClaims.Audience) {
		t.Errorf("Audience length mismatch: %d != %d",
			len(workloadClaims.Audience), len(managementClaims.Audience))
	} else {
		for i := range workloadClaims.Audience {
			if workloadClaims.Audience[i] != managementClaims.Audience[i] {
				t.Errorf("Audience[%d] mismatch: %s != %s",
					i, workloadClaims.Audience[i], managementClaims.Audience[i])
			}
		}
	}

	// Expiration (exp) - MUST be identical
	if !workloadClaims.ExpiresAt.Time.Equal(managementClaims.ExpiresAt.Time) {
		t.Errorf("Expiration mismatch: %v != %v",
			workloadClaims.ExpiresAt.Time, managementClaims.ExpiresAt.Time)
	}

	// Issuer (iss) - MUST be identical
	if workloadClaims.Issuer != managementClaims.Issuer {
		t.Errorf("Issuer mismatch: %s != %s",
			workloadClaims.Issuer, managementClaims.Issuer)
	}

	// Namespace - MUST be identical
	if workloadClaims.Kubernetes.Namespace != managementClaims.Kubernetes.Namespace {
		t.Errorf("Namespace mismatch: %s != %s",
			workloadClaims.Kubernetes.Namespace,
			managementClaims.Kubernetes.Namespace)
	}

	// Service Account Name - MUST be identical
	if workloadClaims.Kubernetes.ServiceAccount.Name != managementClaims.Kubernetes.ServiceAccount.Name {
		t.Errorf("Service account name mismatch: %s != %s",
			workloadClaims.Kubernetes.ServiceAccount.Name,
			managementClaims.Kubernetes.ServiceAccount.Name)
	}

	// ============================================================================
	// VERIFY CLAIMS THAT MUST BE DIFFERENT
	// ============================================================================

	// Issued At (iat) - May be the same or different (tokens can be issued in same second)
	// JWT uses second precision, so they may have the same timestamp
	// What matters is the unique JWT ID (jti)

	// JWT ID (jti) - MUST be different (unique identifier for each token)
	if workloadClaims.ID == managementClaims.ID {
		t.Error("JWT ID (jti) should be different (unique per token)")
	}

	// Service Account UID - MUST be different (workload vs management)
	if workloadClaims.Kubernetes.ServiceAccount.UID == managementClaims.Kubernetes.ServiceAccount.UID {
		t.Error("UIDs should be different (workload vs management)")
	}

	// Not Before (nbf) - May be the same or different (aligned with iat)
	// JWT uses second precision, so they may have the same timestamp

	// ============================================================================
	// VALIDATE UID FORMAT
	// ============================================================================

	// Verify workload UID is valid UUID format
	if !uuidRegex.MatchString(workloadClaims.Kubernetes.ServiceAccount.UID) {
		t.Errorf("Workload UID is not valid UUID format: %s",
			workloadClaims.Kubernetes.ServiceAccount.UID)
	}

	// Verify management UID is valid UUID format
	if !uuidRegex.MatchString(managementClaims.Kubernetes.ServiceAccount.UID) {
		t.Errorf("Management UID is not valid UUID format: %s",
			managementClaims.Kubernetes.ServiceAccount.UID)
	}

	// ============================================================================
	// LOG VERIFICATION RESULTS
	// ============================================================================

	t.Logf("\n--- Workload Token (Authenticated) ---")
	t.Logf("Issuer: %s", workloadClaims.Issuer)
	t.Logf("Subject: %s", workloadClaims.Subject)
	t.Logf("Audience: %v", workloadClaims.Audience)
	t.Logf("Namespace: %s", workloadClaims.Kubernetes.Namespace)
	t.Logf("Service Account: %s", workloadClaims.Kubernetes.ServiceAccount.Name)
	t.Logf("UID: %s", workloadClaims.Kubernetes.ServiceAccount.UID)
	t.Logf("Expiration: %v (Unix: %d)", workloadClaims.ExpiresAt.Time, workloadClaims.ExpiresAt.Unix())
	t.Logf("IssuedAt: %v (Unix: %d)", workloadClaims.IssuedAt.Time, workloadClaims.IssuedAt.Unix())
	t.Logf("JWT ID: %s", workloadClaims.ID)

	t.Logf("\n--- Management Token (Exchanged) ---")
	t.Logf("Issuer: %s", managementClaims.Issuer)
	t.Logf("Subject: %s", managementClaims.Subject)
	t.Logf("Audience: %v", managementClaims.Audience)
	t.Logf("Namespace: %s", managementClaims.Kubernetes.Namespace)
	t.Logf("Service Account: %s", managementClaims.Kubernetes.ServiceAccount.Name)
	t.Logf("UID: %s", managementClaims.Kubernetes.ServiceAccount.UID)
	t.Logf("Expiration: %v (Unix: %d)", managementClaims.ExpiresAt.Time, managementClaims.ExpiresAt.Unix())
	t.Logf("IssuedAt: %v (Unix: %d)", managementClaims.IssuedAt.Time, managementClaims.IssuedAt.Unix())
	t.Logf("JWT ID: %s", managementClaims.ID)

	t.Logf("\n--- Verification ---")
	t.Logf("✓ Subject (sub) matches: %s", workloadClaims.Subject)
	t.Logf("✓ Audience (aud) matches: %v", workloadClaims.Audience)
	t.Logf("✓ Expiration (exp) matches: %v", workloadClaims.ExpiresAt.Time)
	t.Logf("✓ Issuer (iss) matches: %s", workloadClaims.Issuer)
	t.Logf("✓ Namespace matches: %s", workloadClaims.Kubernetes.Namespace)
	t.Logf("✓ Service account name matches: %s", workloadClaims.Kubernetes.ServiceAccount.Name)
	if workloadClaims.IssuedAt.Time.Equal(managementClaims.IssuedAt.Time) {
		t.Logf("  IssuedAt (iat) same second: %v (JWT second precision)", workloadClaims.IssuedAt.Time)
	} else {
		t.Logf("  IssuedAt (iat) differs: workload=%v, management=%v",
			workloadClaims.IssuedAt.Time, managementClaims.IssuedAt.Time)
	}
	t.Logf("✓ JWT ID (jti) differs: workload=%s, management=%s",
		workloadClaims.ID, managementClaims.ID)
	t.Logf("✓ UID differs: workload=%s, management=%s",
		workloadClaims.Kubernetes.ServiceAccount.UID,
		managementClaims.Kubernetes.ServiceAccount.UID)
	if workloadClaims.NotBefore.Time.Equal(managementClaims.NotBefore.Time) {
		t.Logf("  NotBefore (nbf) same second: %v (JWT second precision)", workloadClaims.NotBefore.Time)
	} else {
		t.Logf("  NotBefore (nbf) differs: workload=%v, management=%v",
			workloadClaims.NotBefore.Time, managementClaims.NotBefore.Time)
	}
	t.Logf("✓ Both UIDs are valid UUID format")
}

// TestTokenExchangeServiceAccountNotFound tests the error case when the service
// account doesn't exist in the management cluster
func TestTokenExchangeServiceAccountNotFound(t *testing.T) {
	ctx := context.Background()
	t.Log("Testing service account not found scenario")

	// Setup JWT signer
	issuer := "https://kubernetes.default.svc.cluster.local"
	workloadSigner, err := testutil.NewJWTSigner(issuer)
	if err != nil {
		t.Fatalf("Failed to create workload JWT signer: %v", err)
	}

	// Test data
	namespace := "app-prod"
	saName := "eso-sa"
	workloadUID := "12345678-1234-1234-1234-123456789abc"
	audiences := []string{"https://kubernetes.default.svc"}
	expiration := time.Now().Add(1 * time.Hour)

	// Generate workload cluster token
	workloadToken, err := workloadSigner.GenerateToken(namespace, saName, workloadUID, audiences, expiration)
	if err != nil {
		t.Fatalf("Failed to generate workload token: %v", err)
	}

	// Setup fake workload client with TokenReview reactor
	workloadClient := fake.NewSimpleClientset()
	workloadClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		createAction := action.(k8stesting.CreateAction)
		tr := createAction.GetObject().(*authenticationv1.TokenReview)

		tr.Status = authenticationv1.TokenReviewStatus{
			Authenticated: true,
			User: authenticationv1.UserInfo{
				Username: "system:serviceaccount:" + namespace + ":" + saName,
				UID:      workloadUID,
			},
		}
		return true, tr, nil
	})

	// Setup fake management client WITHOUT the service account
	managementClient := fake.NewSimpleClientset()

	// Create validator and exchanger
	validator := NewValidator(workloadClient)

	expirationSeconds := int64(3600)
	exchanger := NewExchanger(managementClient, ExchangeConfig{
		Audiences:         audiences,
		ExpirationSeconds: &expirationSeconds,
	})

	// Execute validation (should succeed)
	identity, err := validator.Validate(ctx, workloadToken)
	if err != nil {
		t.Fatalf("Token validation failed: %v", err)
	}

	// Execute exchange (should fail - service account not found)
	_, err = exchanger.Exchange(ctx, identity)
	if err == nil {
		t.Error("Expected exchange to fail when service account doesn't exist, but it succeeded")
	}

	// Verify error message contains the service account details
	errMsg := err.Error()
	if !contains(errMsg, namespace) {
		t.Errorf("Error message should contain namespace %q: %s", namespace, errMsg)
	}
	if !contains(errMsg, saName) {
		t.Errorf("Error message should contain service account name %q: %s", saName, errMsg)
	}
	if !contains(errMsg, "not found") {
		t.Errorf("Error message should indicate 'not found': %s", errMsg)
	}

	t.Logf("✓ Exchange correctly failed with: %v", err)
}

// TestTokenValidationFails tests the error case when token validation fails
func TestTokenValidationFails(t *testing.T) {
	ctx := context.Background()
	t.Log("Testing validation failure scenario")

	// Setup fake workload client with TokenReview reactor that returns unauthenticated
	workloadClient := fake.NewSimpleClientset()
	workloadClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		createAction := action.(k8stesting.CreateAction)
		tr := createAction.GetObject().(*authenticationv1.TokenReview)

		// Return unauthenticated
		tr.Status = authenticationv1.TokenReviewStatus{
			Authenticated: false,
			Error:         "token has expired",
		}
		return true, tr, nil
	})

	// Create validator
	validator := NewValidator(workloadClient)

	// Execute validation (should fail)
	_, err := validator.Validate(ctx, "invalid-or-expired-token")
	if err == nil {
		t.Error("Expected validation to fail for invalid token, but it succeeded")
	}

	errMsg := err.Error()
	if !contains(errMsg, "token validation failed") && !contains(errMsg, "not authenticated") {
		t.Errorf("Error message should indicate authentication failure: %s", errMsg)
	}

	t.Logf("✓ Validation correctly failed with: %v", err)
}

// TestTokenExpirationPreserved tests that the expiration time is preserved
// during token exchange
func TestTokenExpirationPreserved(t *testing.T) {
	ctx := context.Background()

	// Setup JWT signers for both clusters
	issuer := "https://kubernetes.default.svc.cluster.local"

	workloadSigner, err := testutil.NewJWTSigner(issuer)
	if err != nil {
		t.Fatalf("Failed to create workload JWT signer: %v", err)
	}

	managementSigner, err := testutil.NewJWTSigner(issuer)
	if err != nil {
		t.Fatalf("Failed to create management JWT signer: %v", err)
	}

	// Test data with specific expiration time (2 hours from now)
	namespace := "default"
	saName := "default"
	workloadUID := "72b0e9c5-c44a-4de0-ae59-9b400f1221e0"
	managementUID := "a1b2c3d4-e5f6-4789-a0b1-c2d3e4f5a6b7"
	audiences := []string{"https://kubernetes.default.svc"}
	expiration := time.Now().Add(2 * time.Hour).Truncate(time.Second) // Truncate to second precision

	// Generate workload cluster token
	workloadToken, err := workloadSigner.GenerateToken(namespace, saName, workloadUID, audiences, expiration)
	if err != nil {
		t.Fatalf("Failed to generate workload token: %v", err)
	}

	// Setup fake workload client
	workloadClient := fake.NewSimpleClientset()
	workloadClient.PrependReactor("create", "tokenreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		createAction := action.(k8stesting.CreateAction)
		tr := createAction.GetObject().(*authenticationv1.TokenReview)

		tr.Status = authenticationv1.TokenReviewStatus{
			Authenticated: true,
			User: authenticationv1.UserInfo{
				Username: "system:serviceaccount:" + namespace + ":" + saName,
				UID:      workloadUID,
			},
		}
		return true, tr, nil
	})

	// Setup fake management client
	managementClient := fake.NewSimpleClientset()

	// Add service account
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: namespace,
			UID:       types.UID(managementUID),
		},
	}
	_, err = managementClient.CoreV1().ServiceAccounts(namespace).Create(ctx, sa, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create service account: %v", err)
	}

	// Add reactor to mock TokenRequest with same expiration
	managementClient.PrependReactor("create", "serviceaccounts", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "token" {
			createAction := action.(k8stesting.CreateAction)
			tokenReq := createAction.GetObject().(*authenticationv1.TokenRequest)

			// Generate JWT with same expiration as input token
			jwtToken, err := managementSigner.GenerateToken(
				namespace,
				saName,
				managementUID,
				tokenReq.Spec.Audiences,
				expiration, // Same expiration as input
			)
			if err != nil {
				return true, nil, err
			}

			tokenReq.Status = authenticationv1.TokenRequestStatus{
				Token:               jwtToken,
				ExpirationTimestamp: metav1.Time{Time: expiration},
			}
			return true, tokenReq, nil
		}
		return false, nil, nil
	})

	// Create validator and exchanger
	validator := NewValidator(workloadClient)

	expirationSeconds := int64(7200) // 2 hours
	exchanger := NewExchanger(managementClient, ExchangeConfig{
		Audiences:         audiences,
		ExpirationSeconds: &expirationSeconds,
	})

	// Execute validation and exchange
	identity, err := validator.Validate(ctx, workloadToken)
	if err != nil {
		t.Fatalf("Token validation failed: %v", err)
	}

	metadata, err := exchanger.ExchangeWithMetadata(ctx, identity)
	if err != nil {
		t.Fatalf("Token exchange failed: %v", err)
	}

	// Parse both tokens
	workloadClaims, err := workloadSigner.ParseToken(workloadToken)
	if err != nil {
		t.Fatalf("Failed to parse workload token: %v", err)
	}

	managementClaims, err := managementSigner.ParseToken(metadata.Token)
	if err != nil {
		t.Fatalf("Failed to parse management token: %v", err)
	}

	// Verify expiration times match exactly
	if !workloadClaims.ExpiresAt.Time.Equal(managementClaims.ExpiresAt.Time) {
		t.Errorf("Expiration times should match:\n  Workload:   %v (Unix: %d)\n  Management: %v (Unix: %d)",
			workloadClaims.ExpiresAt.Time, workloadClaims.ExpiresAt.Unix(),
			managementClaims.ExpiresAt.Time, managementClaims.ExpiresAt.Unix())
	}

	// Also verify metadata expiration matches
	if !metadata.ExpirationTime.Equal(workloadClaims.ExpiresAt.Time) {
		t.Errorf("Metadata expiration time should match token claim:\n  Metadata: %v\n  Token:    %v",
			metadata.ExpirationTime, workloadClaims.ExpiresAt.Time)
	}

	t.Logf("\n--- Expiration Verification ---")
	t.Logf("Input expiration:  %v (Unix: %d)", workloadClaims.ExpiresAt.Time, workloadClaims.ExpiresAt.Unix())
	t.Logf("Output expiration: %v (Unix: %d)", managementClaims.ExpiresAt.Time, managementClaims.ExpiresAt.Unix())
	t.Logf("✓ Timestamps match exactly")
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

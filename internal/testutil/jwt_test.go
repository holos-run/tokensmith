package testutil

import (
	"regexp"
	"testing"
	"time"
)

// UUID validation regex
var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func TestJWTTokenRoundTrip(t *testing.T) {
	t.Log("Testing JWT generation and parsing")

	// Create JWT signer
	signer, err := NewJWTSigner("https://test-cluster.example.com")
	if err != nil {
		t.Fatalf("Failed to create JWT signer: %v", err)
	}

	// Generate token
	namespace := "default"
	name := "test-sa"
	uid := "72b0e9c5-c44a-4de0-ae59-9b400f1221e0"
	audiences := []string{"https://kubernetes.default.svc"}
	expiration := time.Now().Add(1 * time.Hour)

	tokenString, err := signer.GenerateToken(namespace, name, uid, audiences, expiration)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}
	t.Log("✓ Generated valid JWT token")

	// Parse token
	claims, err := signer.ParseToken(tokenString)
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}
	t.Log("✓ Parsed JWT successfully")

	// Verify claims
	if claims.Kubernetes.Namespace != namespace {
		t.Errorf("Namespace claim mismatch: got %q, want %q", claims.Kubernetes.Namespace, namespace)
	} else {
		t.Logf("✓ Namespace claim matches: %s", namespace)
	}

	if claims.Kubernetes.ServiceAccount.Name != name {
		t.Errorf("Service account claim mismatch: got %q, want %q", claims.Kubernetes.ServiceAccount.Name, name)
	} else {
		t.Logf("✓ Service account claim matches: %s", name)
	}

	if claims.Kubernetes.ServiceAccount.UID != uid {
		t.Errorf("UID claim mismatch: got %q, want %q", claims.Kubernetes.ServiceAccount.UID, uid)
	} else {
		t.Logf("✓ UID claim matches: %s", uid)
	}

	// Verify subject
	expectedSubject := "system:serviceaccount:" + namespace + ":" + name
	if claims.Subject != expectedSubject {
		t.Errorf("Subject mismatch: got %q, want %q", claims.Subject, expectedSubject)
	} else {
		t.Logf("✓ Subject matches: %s", expectedSubject)
	}

	// Verify audience
	if len(claims.Audience) != len(audiences) {
		t.Errorf("Audience length mismatch: got %d, want %d", len(claims.Audience), len(audiences))
	} else {
		for i := range audiences {
			if claims.Audience[i] != audiences[i] {
				t.Errorf("Audience[%d] mismatch: got %q, want %q", i, claims.Audience[i], audiences[i])
			}
		}
		t.Logf("✓ Audience matches: %v", audiences)
	}

	// Verify issuer
	if claims.Issuer != signer.issuer {
		t.Errorf("Issuer mismatch: got %q, want %q", claims.Issuer, signer.issuer)
	} else {
		t.Logf("✓ Issuer matches: %s", signer.issuer)
	}

	// Verify JWT ID is a valid UUID
	if !uuidRegex.MatchString(claims.ID) {
		t.Errorf("JWT ID is not a valid UUID: %s", claims.ID)
	} else {
		t.Logf("✓ JWT ID is valid UUID: %s", claims.ID)
	}

	// Verify expiration is approximately correct (within 1 second tolerance)
	expectedExp := expiration.Unix()
	actualExp := claims.ExpiresAt.Unix()
	if abs(expectedExp-actualExp) > 1 {
		t.Errorf("Expiration mismatch: got %d, want %d", actualExp, expectedExp)
	} else {
		t.Logf("✓ Expiration matches: %v", claims.ExpiresAt.Time)
	}
}

func TestJWTWithDifferentKeys(t *testing.T) {
	// Create two different JWT signers
	signer1, err := NewJWTSigner("https://cluster1.example.com")
	if err != nil {
		t.Fatalf("Failed to create signer1: %v", err)
	}

	signer2, err := NewJWTSigner("https://cluster2.example.com")
	if err != nil {
		t.Fatalf("Failed to create signer2: %v", err)
	}

	// Generate token with signer1
	tokenString, err := signer1.GenerateToken(
		"default",
		"test-sa",
		"test-uid-123",
		[]string{"https://kubernetes.default.svc"},
		time.Now().Add(1*time.Hour),
	)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Try to parse with signer1 (should succeed)
	_, err = signer1.ParseToken(tokenString)
	if err != nil {
		t.Errorf("Expected signer1 to parse its own token, got error: %v", err)
	} else {
		t.Log("✓ Signer1 successfully parsed its own token")
	}

	// Try to parse with signer2 (should fail - different key)
	_, err = signer2.ParseToken(tokenString)
	if err == nil {
		t.Error("Expected signer2 to fail parsing signer1's token, but it succeeded")
	} else {
		t.Logf("✓ Signer2 correctly failed to parse signer1's token: %v", err)
	}
}

func TestJWTUniqueTokenIDs(t *testing.T) {
	signer, err := NewJWTSigner("https://test-cluster.example.com")
	if err != nil {
		t.Fatalf("Failed to create JWT signer: %v", err)
	}

	// Generate two tokens for the same service account
	token1, err := signer.GenerateToken(
		"default",
		"test-sa",
		"test-uid-123",
		[]string{"https://kubernetes.default.svc"},
		time.Now().Add(1*time.Hour),
	)
	if err != nil {
		t.Fatalf("Failed to generate token1: %v", err)
	}

	// Small delay to ensure different issued-at times (JWT uses second precision)
	time.Sleep(1100 * time.Millisecond)

	token2, err := signer.GenerateToken(
		"default",
		"test-sa",
		"test-uid-123",
		[]string{"https://kubernetes.default.svc"},
		time.Now().Add(1*time.Hour),
	)
	if err != nil {
		t.Fatalf("Failed to generate token2: %v", err)
	}

	// Parse both tokens
	claims1, err := signer.ParseToken(token1)
	if err != nil {
		t.Fatalf("Failed to parse token1: %v", err)
	}

	claims2, err := signer.ParseToken(token2)
	if err != nil {
		t.Fatalf("Failed to parse token2: %v", err)
	}

	// Verify JWT IDs are different
	if claims1.ID == claims2.ID {
		t.Error("Expected different JWT IDs (jti), but they are identical")
	} else {
		t.Logf("✓ JWT IDs are unique: token1=%s, token2=%s", claims1.ID, claims2.ID)
	}

	// Verify both are valid UUIDs
	if !uuidRegex.MatchString(claims1.ID) {
		t.Errorf("Token1 JWT ID is not a valid UUID: %s", claims1.ID)
	}
	if !uuidRegex.MatchString(claims2.ID) {
		t.Errorf("Token2 JWT ID is not a valid UUID: %s", claims2.ID)
	}
	if uuidRegex.MatchString(claims1.ID) && uuidRegex.MatchString(claims2.ID) {
		t.Log("✓ Both JWT IDs are valid UUIDs")
	}

	// Verify issued-at times are different
	if claims1.IssuedAt.Time.Equal(claims2.IssuedAt.Time) {
		t.Error("Expected different IssuedAt times, but they are identical")
	} else {
		t.Logf("✓ IssuedAt times differ: token1=%v, token2=%v",
			claims1.IssuedAt.Time, claims2.IssuedAt.Time)
	}
}

// Helper function for absolute value
func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

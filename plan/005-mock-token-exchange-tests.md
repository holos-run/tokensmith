# Plan 005: Mock Token Exchange Unit Tests

**Status**: Draft

## Overview

Add comprehensive unit tests for the token exchange flow using Kubernetes fake clients. These tests will mock the Kubernetes API to validate the complete token validation and exchange logic without requiring actual cluster access.

## Goals

1. Create unit tests that can be run locally without Kubernetes clusters
2. Use standard Kubernetes testing conventions (`k8s.io/client-go/kubernetes/fake`)
3. Test the complete exchange flow: validate workload token → exchange for management token
4. Verify token claims are preserved correctly (namespace, name, audience, expiration)
5. Provide clear, human-readable test output showing what was verified
6. Document how to run tests and interpret results

## Background: Kubernetes Service Account Token Format

Kubernetes service account tokens are JWTs with the following standard claims:

```json
{
  "aud": [
    "https://kubernetes.default.svc.cluster.local",
    "k3s"
  ],
  "exp": 1762582177,
  "iat": 1762578577,
  "iss": "https://kubernetes.default.svc.cluster.local",
  "jti": "94aed96e-d715-49fd-9c79-11409d289871",
  "kubernetes.io": {
    "namespace": "default",
    "serviceaccount": {
      "name": "default",
      "uid": "72b0e9c5-c44a-4de0-ae59-9b400f1221e0"
    }
  },
  "nbf": 1762578577,
  "sub": "system:serviceaccount:default:default"
}

```

**Key Claims:**
- `aud`: Audience - typically `["https://kubernetes.default.svc"]` for API server access
- `sub`: Subject - format `system:serviceaccount:<namespace>:<name>`
- `exp`: Expiration timestamp (Unix epoch)
- `iat`: Issued at timestamp (Unix epoch)
- `iss`: Issuer - the API server URL
- Custom claims for namespace, name, and UID

## Test Scenarios

### Scenario 1: Successful Token Exchange
**Setup:**
- Workload cluster has service account `default/default` with UID `workload-uid-123`
- Management cluster has service account `default/default` with UID `management-uid-456`
- Input token is valid with audience `https://kubernetes.default.svc`

**Test Flow:**
1. Mock TokenReview response (workload cluster) returns authenticated=true
2. Mock TokenRequest response (management cluster) returns new token
3. Verify exchange succeeds
4. Parse both tokens and verify:
   - Namespace matches: `default`
   - Service account name matches: `default`
   - Audience matches: `https://kubernetes.default.svc`
   - Expiration time matches (same Unix timestamp)
   - UIDs are different (workload vs management)

**Success Criteria:**
- Test passes with clear output showing what was verified
- Human can see the comparison of token claims

### Scenario 2: Service Account Not Found in Management Cluster
**Setup:**
- Workload cluster has service account `app-prod/eso-sa`
- Management cluster does NOT have matching service account

**Test Flow:**
1. Mock TokenReview returns authenticated=true for `app-prod/eso-sa`
2. Mock ServiceAccount GET returns NotFound error
3. Verify exchange fails with appropriate error message

**Success Criteria:**
- Test verifies error message contains service account name and namespace
- Error indicates service account doesn't exist in management cluster

### Scenario 3: Invalid Token from Workload Cluster
**Setup:**
- Invalid/expired token provided

**Test Flow:**
1. Mock TokenReview returns authenticated=false
2. Verify validation fails before attempting exchange

**Success Criteria:**
- Validation fails with appropriate error
- No TokenRequest attempted

### Scenario 4: Token Expiration Preservation
**Setup:**
- Input token has specific expiration time (e.g., 1 hour from now)
- Exchange should preserve the same expiration

**Test Flow:**
1. Mock TokenReview with token expiring at specific time
2. Configure exchanger to use same expiration duration
3. Mock TokenRequest to return token with same expiration
4. Verify both tokens have identical `exp` claim

**Success Criteria:**
- Expiration timestamps match exactly
- Test output clearly shows both expiration times

## Implementation Approach

### 1. Using Kubernetes Fake Clients

```go
import (
    "k8s.io/client-go/kubernetes/fake"
    "k8s.io/client-go/testing"
)

// Create fake workload client
workloadClient := fake.NewSimpleClientset()

// Add reactor to mock TokenReview
workloadClient.PrependReactor("create", "tokenreviews", func(action testing.Action) (bool, runtime.Object, error) {
    createAction := action.(testing.CreateAction)
    tr := createAction.GetObject().(*authenticationv1.TokenReview)

    // Mock the response
    tr.Status = authenticationv1.TokenReviewStatus{
        Authenticated: true,
        User: authenticationv1.UserInfo{
            Username: "system:serviceaccount:default:default",
            UID:      "workload-uid-123",
        },
    }
    return true, tr, nil
})

// Create fake management client
managementClient := fake.NewSimpleClientset()

// Add service account to management cluster
sa := &corev1.ServiceAccount{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "default",
        Namespace: "default",
        UID:       "management-uid-456",
    },
}
managementClient.CoreV1().ServiceAccounts("default").Create(context.Background(), sa, metav1.CreateOptions{})

// Add reactor to mock TokenRequest
managementClient.PrependReactor("create", "serviceaccounts", func(action testing.Action) (bool, runtime.Object, error) {
    // Check if this is a token request (subresource)
    if action.GetSubresource() == "token" {
        createAction := action.(testing.CreateAction)
        tokenReq := createAction.GetObject().(*authenticationv1.TokenRequest)

        // Mock the response with a token
        tokenReq.Status = authenticationv1.TokenRequestStatus{
            Token: "mock-management-token",
            ExpirationTimestamp: metav1.Time{Time: time.Now().Add(1 * time.Hour)},
        }
        return true, tokenReq, nil
    }
    return false, nil, nil
})
```

### 2. Test Structure

Create `internal/token/exchange_integration_test.go`:

```go
package token

import (
    "context"
    "testing"
    "time"

    authenticationv1 "k8s.io/api/authentication/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/client-go/kubernetes/fake"
    "k8s.io/client-go/testing"
)

func TestTokenExchangeFlow(t *testing.T) {
    // Test implementation
}

func TestTokenExchangeServiceAccountNotFound(t *testing.T) {
    // Test implementation
}

func TestTokenValidationFails(t *testing.T) {
    // Test implementation
}

func TestTokenExpirationPreserved(t *testing.T) {
    // Test implementation
}
```

### 3. JWT Token Generation and Verification

**Approach: Generate actual valid Kubernetes service account JWTs**

Following Kubernetes upstream testing patterns, we will:
1. Generate RSA key pairs for signing tokens
2. Create valid JWTs with proper Kubernetes service account claims
3. Parse and verify JWT tokens in tests
4. Compare token claims to ensure correctness

This mirrors how Kubernetes itself tests service account tokens and provides the most realistic test coverage.

#### JWT Token Generator

Create a test helper to generate valid Kubernetes service account tokens:

```go
// testutil/jwt.go
package testutil

import (
    "crypto/rand"
    "crypto/rsa"
    "time"

    "github.com/golang-jwt/jwt/v5"
)

// TokenClaims represents Kubernetes service account token claims
type TokenClaims struct {
    jwt.RegisteredClaims
    Kubernetes KubernetesClaims `json:"kubernetes.io"`
}

type KubernetesClaims struct {
    Namespace      string             `json:"namespace"`
    ServiceAccount ServiceAccountInfo `json:"serviceaccount"`
}

type ServiceAccountInfo struct {
    Name string `json:"name"`
    UID  string `json:"uid"`
}

// JWTSigner generates and signs Kubernetes service account tokens
type JWTSigner struct {
    privateKey *rsa.PrivateKey
    publicKey  *rsa.PublicKey
    issuer     string
}

// NewJWTSigner creates a new JWT signer with a generated RSA key pair
func NewJWTSigner(issuer string) (*JWTSigner, error) {
    privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
    if err != nil {
        return nil, err
    }

    return &JWTSigner{
        privateKey: privateKey,
        publicKey:  &privateKey.PublicKey,
        issuer:     issuer,
    }, nil
}

// GenerateToken creates a valid Kubernetes service account JWT
func (s *JWTSigner) GenerateToken(namespace, name, uid string, audiences []string, expiration time.Time) (string, error) {
    now := time.Now()

    claims := TokenClaims{
        RegisteredClaims: jwt.RegisteredClaims{
            Issuer:    s.issuer,
            Subject:   "system:serviceaccount:" + namespace + ":" + name,
            Audience:  audiences,
            ExpiresAt: jwt.NewNumericDate(expiration),
            IssuedAt:  jwt.NewNumericDate(now),
            NotBefore: jwt.NewNumericDate(now),
        },
        Kubernetes: KubernetesClaims{
            Namespace: namespace,
            ServiceAccount: ServiceAccountInfo{
                Name: name,
                UID:  uid,
            },
        },
    }

    token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
    return token.SignedString(s.privateKey)
}

// ParseToken parses and validates a JWT token
func (s *JWTSigner) ParseToken(tokenString string) (*TokenClaims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
        return s.publicKey, nil
    })

    if err != nil {
        return nil, err
    }

    if claims, ok := token.Claims.(*TokenClaims); ok && token.Valid {
        return claims, nil
    }

    return nil, jwt.ErrTokenInvalidClaims
}
```

#### Mock Token Generation in Reactors

Update the mock reactors to generate actual JWT tokens:

```go
// Setup workload cluster JWT signer
workloadSigner, err := testutil.NewJWTSigner("https://workload-cluster.example.com")
if err != nil {
    t.Fatal(err)
}

// Setup management cluster JWT signer
managementSigner, err := testutil.NewJWTSigner("https://management-cluster.example.com")
if err != nil {
    t.Fatal(err)
}

// Mock TokenRequest to return actual JWT
managementClient.PrependReactor("create", "serviceaccounts", func(action testing.Action) (bool, runtime.Object, error) {
    if action.GetSubresource() == "token" {
        createAction := action.(testing.CreateAction)
        tokenReq := createAction.GetObject().(*authenticationv1.TokenRequest)

        // Generate actual JWT token
        expiration := time.Now().Add(1 * time.Hour)
        jwtToken, err := managementSigner.GenerateToken(
            "default",
            "default",
            "management-uid-456",
            tokenReq.Spec.Audiences,
            expiration,
        )
        if err != nil {
            return true, nil, err
        }

        tokenReq.Status = authenticationv1.TokenRequestStatus{
            Token: jwtToken,
            ExpirationTimestamp: metav1.Time{Time: expiration},
        }
        return true, tokenReq, nil
    }
    return false, nil, nil
})
```

#### Token Verification in Tests

Parse and verify both input and output tokens:

```go
// Parse workload token (input)
workloadClaims, err := workloadSigner.ParseToken(workloadToken)
if err != nil {
    t.Fatalf("Failed to parse workload token: %v", err)
}

// Parse management token (output)
managementClaims, err := managementSigner.ParseToken(managementToken)
if err != nil {
    t.Fatalf("Failed to parse management token: %v", err)
}

// Verify claims match
if workloadClaims.Kubernetes.Namespace != managementClaims.Kubernetes.Namespace {
    t.Errorf("Namespace mismatch: %s != %s",
        workloadClaims.Kubernetes.Namespace,
        managementClaims.Kubernetes.Namespace)
}

if workloadClaims.Kubernetes.ServiceAccount.Name != managementClaims.Kubernetes.ServiceAccount.Name {
    t.Errorf("Service account name mismatch: %s != %s",
        workloadClaims.Kubernetes.ServiceAccount.Name,
        managementClaims.Kubernetes.ServiceAccount.Name)
}

// Verify expiration matches
if !workloadClaims.ExpiresAt.Equal(*managementClaims.ExpiresAt) {
    t.Errorf("Expiration mismatch: %v != %v",
        workloadClaims.ExpiresAt, managementClaims.ExpiresAt)
}

// Verify UIDs are different
if workloadClaims.Kubernetes.ServiceAccount.UID == managementClaims.Kubernetes.ServiceAccount.UID {
    t.Error("UIDs should be different (workload vs management)")
}
```

### 4. Test Output Format

Tests should produce clear, human-readable output showing JWT claim verification:

```
=== RUN   TestTokenExchangeFlow
    exchange_integration_test.go:78:
        --- Workload Token (Input) ---
        Issuer: https://workload-cluster.example.com
        Subject: system:serviceaccount:default:default
        Audience: [https://kubernetes.default.svc]
        Namespace: default
        Service Account: default
        UID: workload-uid-123
        Expiration: 2024-12-31T12:00:00Z (Unix: 1735660800)

        --- Management Token (Output) ---
        Issuer: https://management-cluster.example.com
        Subject: system:serviceaccount:default:default
        Audience: [https://kubernetes.default.svc]
        Namespace: default
        Service Account: default
        UID: management-uid-456
        Expiration: 2024-12-31T12:00:00Z (Unix: 1735660800)

        --- Verification ---
        ✓ Namespace matches: default
        ✓ Service account name matches: default
        ✓ Audience matches: [https://kubernetes.default.svc]
        ✓ Expiration matches: 2024-12-31T12:00:00Z
        ✓ UIDs are different (workload-uid-123 vs management-uid-456)
        ✓ Both tokens are valid JWTs

--- PASS: TestTokenExchangeFlow (0.00s)
```

## Implementation Steps

### Phase 1: Create JWT Token Generator

1. Create `internal/testutil/jwt.go`
   - Implement `JWTSigner` struct with RSA key pair generation
   - Implement `GenerateToken()` - creates valid Kubernetes service account JWTs
   - Implement `ParseToken()` - parses and validates JWT tokens
   - Include proper Kubernetes claims structure matching actual K8s tokens

2. Create `internal/testutil/jwt_test.go`
   - Test JWT generation creates valid tokens
   - Test JWT parsing correctly extracts claims
   - Test token validation with correct/incorrect keys
   - Verify claim structure matches Kubernetes format

### Phase 2: Setup Test Infrastructure

1. Create `internal/token/exchange_integration_test.go`
2. Add test helper functions:
   - `setupTestEnvironment()` - creates JWT signers for both clusters
   - `createFakeWorkloadClient()` - returns fake client with TokenReview reactor
   - `createFakeManagementClient()` - returns fake client with service accounts and TokenRequest reactor
   - `generateWorkloadToken()` - creates a valid workload cluster JWT
   - `mockTokenReview()` - configures TokenReview reactor to validate input JWT
   - `mockTokenRequest()` - configures TokenRequest reactor to return valid JWT

### Phase 3: Implement Test Scenarios

1. **TestTokenExchangeFlow** - Happy path test
   - Setup JWT signers for workload and management clusters
   - Generate valid workload JWT for `default/default` service account
   - Setup fake clients with JWT-generating reactors
   - Create validator and exchanger
   - Execute validation
   - Execute exchange using `ExchangeWithMetadata()`
   - Parse both input and output JWTs
   - Verify JWT claims:
     - Namespace: `default` (matches)
     - Service account: `default` (matches)
     - Audience: `https://kubernetes.default.svc` (matches)
     - Expiration timestamp (matches)
     - UIDs are different (workload vs management)
   - Print detailed comparison of JWT claims

2. **TestTokenExchangeServiceAccountNotFound**
   - Setup JWT signers
   - Generate valid workload JWT for `app-prod/eso-sa`
   - Setup fake workload client (validates token successfully)
   - Setup fake management client WITHOUT matching service account
   - Verify exchange fails with NotFound error
   - Verify error message contains namespace and service account name

3. **TestTokenValidationFails**
   - Setup JWT signers
   - Generate valid workload JWT
   - Setup fake workload client to return authenticated=false
   - Verify validation fails before exchange is attempted
   - Verify appropriate error returned

4. **TestTokenExpirationPreserved**
   - Setup JWT signers
   - Define specific expiration time (e.g., `time.Now().Add(2 * time.Hour)`)
   - Generate workload JWT with specific expiration
   - Configure exchanger to use same expiration duration
   - Setup mock TokenRequest to generate JWT with same expiration
   - Execute validation and exchange
   - Parse both JWTs
   - Compare expiration claims (should match to the second)
   - Print both expiration times with Unix timestamps for verification

5. **TestJWTTokenRoundTrip**
   - Verify that generated tokens can be parsed correctly
   - Generate token with known claims
   - Parse the generated token
   - Verify all claims are present and correct
   - Ensures JWT generation/parsing is working correctly

### Phase 4: Documentation and Usage

1. Add section to README.md:

```markdown
## Running Tests

### Unit Tests

Run all unit tests:
```bash
go test ./...
```

Run token exchange tests with verbose output:
```bash
go test -v ./internal/token/
```

Run a specific test:
```bash
go test -v ./internal/token/ -run TestTokenExchangeFlow
```

### Test Coverage

Generate test coverage report:
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

View coverage for a specific package:
```bash
go test -cover ./internal/token/
```
```

2. Add comment to test file explaining the test approach:

```go
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
```

### Phase 5: Verification

1. Run tests and verify output:
```bash
go test -v ./internal/token/
```

2. Check test coverage:
```bash
go test -cover ./internal/token/
```

3. Verify tests can be run individually:
```bash
go test -v ./internal/token/ -run TestTokenExchangeFlow
```

4. Document example test run in plan with expected output

## Success Criteria

1. ✅ All tests pass with `go test ./internal/token/`
2. ✅ Tests generate valid Kubernetes service account JWTs
3. ✅ Tests parse and verify JWT claims correctly
4. ✅ Tests can be run individually with `-run` flag
5. ✅ Test output clearly shows JWT claims comparison
6. ✅ Tests verify JWT claims match (namespace, name, audience, expiration)
7. ✅ Tests verify UIDs are different (workload vs management)
8. ✅ Tests verify expiration timestamps match exactly
9. ✅ Error cases are tested (service account not found, validation fails)
10. ✅ README documents how to run tests
11. ✅ Test coverage is >80% for token package
12. ✅ Tests use standard Kubernetes fake client conventions
13. ✅ JWT generation/parsing helpers have their own unit tests

## Example Test Output

```bash
$ go test -v ./internal/token/

=== RUN   TestJWTTokenRoundTrip
    jwt_test.go:25: Testing JWT generation and parsing
    jwt_test.go:42: ✓ Generated valid JWT token
    jwt_test.go:48: ✓ Parsed JWT successfully
    jwt_test.go:52: ✓ Namespace claim matches: default
    jwt_test.go:56: ✓ Service account claim matches: default
    jwt_test.go:60: ✓ UID claim matches: test-uid-123
--- PASS: TestJWTTokenRoundTrip (0.01s)

=== RUN   TestTokenExchangeFlow
    exchange_integration_test.go:78:
        --- Workload Token (Input) ---
        Issuer: https://workload-cluster.example.com
        Subject: system:serviceaccount:default:default
        Audience: [https://kubernetes.default.svc]
        Namespace: default
        Service Account: default
        UID: workload-uid-123
        Expiration: 2024-12-31T12:00:00Z (Unix: 1735660800)

        --- Management Token (Output) ---
        Issuer: https://management-cluster.example.com
        Subject: system:serviceaccount:default:default
        Audience: [https://kubernetes.default.svc]
        Namespace: default
        Service Account: default
        UID: management-uid-456
        Expiration: 2024-12-31T12:00:00Z (Unix: 1735660800)

        --- Verification ---
        ✓ Namespace matches: default
        ✓ Service account name matches: default
        ✓ Audience matches: [https://kubernetes.default.svc]
        ✓ Expiration matches: 2024-12-31T12:00:00Z
        ✓ UIDs are different (workload-uid-123 vs management-uid-456)
        ✓ Both tokens are valid JWTs
--- PASS: TestTokenExchangeFlow (0.02s)

=== RUN   TestTokenExchangeServiceAccountNotFound
    exchange_integration_test.go:145: Testing service account not found scenario
--- PASS: TestTokenExchangeServiceAccountNotFound (0.00s)

=== RUN   TestTokenValidationFails
    exchange_integration_test.go:178: Testing validation failure scenario
--- PASS: TestTokenValidationFails (0.00s)

=== RUN   TestTokenExpirationPreserved
    exchange_integration_test.go:120:
        --- Expiration Verification ---
        Input expiration:  2024-12-31T14:00:00Z (Unix: 1735660800)
        Output expiration: 2024-12-31T14:00:00Z (Unix: 1735660800)
        ✓ Timestamps match exactly
--- PASS: TestTokenExpirationPreserved (0.00s)

PASS
coverage: 85.2% of statements
ok      github.com/holos-run/tokensmith/internal/token  0.234s
```

## Technical Details

### Standard Kubernetes Service Account Token Audience

The standard audience for Kubernetes service account tokens is:
- **Primary**: `https://kubernetes.default.svc`
- **Alternate**: The API server URL (e.g., `https://kubernetes.default.svc.cluster.local`)

For our tests, we'll use `https://kubernetes.default.svc` as this is the most common and recommended value.

### Token Metadata Comparison

We'll compare these fields between input and output tokens:
- **Must Match**:
  - Namespace
  - Service Account Name
  - Audience
  - Expiration Time (Unix timestamp)

- **Must Be Different**:
  - Service Account UID (workload UID vs management UID)

### Why This Approach

1. **Authentic**: Generates actual valid Kubernetes service account JWTs, not mock strings
2. **Realistic**: Uses actual Kubernetes client-go fake clients with JWT tokens
3. **Standard**: Follows Kubernetes upstream testing patterns for service account tokens
4. **Fast**: No external dependencies or cluster access needed
5. **Verifiable**: Human can see JWT claims and verify correctness
6. **Maintainable**: Uses same structs and types as production code
7. **Comprehensive**: Tests both happy path and error cases with real token parsing

## Dependencies

All dependencies already in go.mod - no new dependencies needed:
- `k8s.io/client-go/kubernetes/fake` - Fake Kubernetes client for testing
- `k8s.io/client-go/testing` - Testing utilities and reactors
- `k8s.io/api/core/v1` - Core Kubernetes API types
- `k8s.io/api/authentication/v1` - Authentication API types (TokenReview, TokenRequest)
- `github.com/golang-jwt/jwt/v5` - JWT generation and parsing (already added in plan 004)

The JWT library is already included for OIDC token operations, so no additional dependencies are required.

## Future Enhancements

1. Add benchmark tests for JWT generation/parsing performance
2. Add table-driven tests for multiple service account combinations
3. Add tests for different audience values
4. Add tests for token caching (when implemented)
5. Add fuzz tests for malformed JWT input handling
6. Add tests for JWT signature verification with incorrect keys
7. Add performance tests for high-volume token exchange scenarios

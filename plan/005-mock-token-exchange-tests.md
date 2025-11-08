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
  "aud": ["https://kubernetes.default.svc"],
  "exp": 1735689600,
  "iat": 1735686000,
  "iss": "https://kubernetes.default.svc.cluster.local",
  "sub": "system:serviceaccount:default:default",
  "kubernetes.io/serviceaccount/namespace": "default",
  "kubernetes.io/serviceaccount/service-account.name": "default",
  "kubernetes.io/serviceaccount/service-account.uid": "12345678-1234-1234-1234-123456789abc"
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

### 3. Token Claim Verification

For verifying token claims, we need to parse the JWT tokens. Since the mock returns string tokens, we have two options:

**Option A: Use actual JWT generation in mocks**
- Generate real JWTs in the mock responses
- Parse and verify claims in tests
- More realistic but requires JWT signing/verification

**Option B: Return structured data instead of string tokens**
- Modify Exchanger to return `TokenMetadata` (already exists)
- Verify metadata fields directly
- Simpler and faster for unit tests

**Recommendation: Use Option B for unit tests**
- Keep tests simple and fast
- No need for JWT parsing overhead
- Can still verify all the important fields
- Add E2E tests later with real JWT tokens if needed

### 4. Test Output Format

Tests should produce clear, human-readable output:

```
=== RUN   TestTokenExchangeFlow
--- Verification Results ---
✓ Service account identity validated
  - Namespace: default
  - Name: default
  - Username: system:serviceaccount:default:default

✓ Token exchange successful
  - Workload SA UID: workload-uid-123
  - Management SA UID: management-uid-456

✓ Token metadata verified
  - Namespace: default (matches)
  - Service Account: default (matches)
  - Expiration: 2024-12-31T12:00:00Z (matches)

--- PASS: TestTokenExchangeFlow (0.00s)
```

## Implementation Steps

### Phase 1: Setup Test Infrastructure

1. Create `internal/token/exchange_integration_test.go`
2. Add test helper functions:
   - `createFakeWorkloadClient()` - returns fake client with TokenReview reactor
   - `createFakeManagementClient()` - returns fake client with service accounts and TokenRequest reactor
   - `setupTestServiceAccount()` - creates test service account in fake client
   - `mockTokenReview()` - configures TokenReview reactor with test data
   - `mockTokenRequest()` - configures TokenRequest reactor with test data

### Phase 2: Implement Test Scenarios

1. **TestTokenExchangeFlow** - Happy path test
   - Setup fake clients
   - Create validator and exchanger
   - Execute validation
   - Execute exchange
   - Verify metadata (using `ExchangeWithMetadata`)
   - Print verification results

2. **TestTokenExchangeServiceAccountNotFound**
   - Setup fake workload client (with valid token)
   - Setup fake management client (without service account)
   - Verify exchange fails
   - Verify error message is descriptive

3. **TestTokenValidationFails**
   - Setup fake workload client (returns authenticated=false)
   - Verify validation fails
   - Verify appropriate error returned

4. **TestTokenExpirationPreserved**
   - Setup with specific expiration time (e.g., `time.Now().Add(2 * time.Hour)`)
   - Configure exchanger with matching expiration
   - Verify returned token has same expiration timestamp
   - Print both timestamps for human verification

### Phase 3: Documentation and Usage

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
// fake Kubernetes clients. These tests verify that:
//
// 1. Token validation correctly parses service account identity
// 2. Token exchange creates tokens for the correct service account
// 3. Metadata is preserved (namespace, name, expiration)
// 4. Errors are handled appropriately
//
// Run with: go test -v ./internal/token/
```

### Phase 4: Verification

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
2. ✅ Tests can be run individually with `-run` flag
3. ✅ Test output clearly shows what was verified
4. ✅ Tests verify token metadata matches (namespace, name, audience, expiration)
5. ✅ Tests verify UIDs are different (workload vs management)
6. ✅ Tests verify expiration timestamps match
7. ✅ Error cases are tested (service account not found, validation fails)
8. ✅ README documents how to run tests
9. ✅ Test coverage is >80% for token package
10. ✅ Tests use standard Kubernetes fake client conventions

## Example Test Output

```bash
$ go test -v ./internal/token/

=== RUN   TestTokenExchangeFlow
    exchange_integration_test.go:45:
        --- Token Exchange Verification ---
        Input Token:
          Namespace: default
          Service Account: default
          Workload UID: workload-uid-123

        Exchanged Token:
          Namespace: default
          Service Account: default
          Management UID: management-uid-456
          Expiration: 2024-12-31T12:00:00Z

        Verification:
          ✓ Namespace matches: default
          ✓ Service account name matches: default
          ✓ UIDs are different (workload vs management)
          ✓ Expiration preserved: 2024-12-31T12:00:00Z
--- PASS: TestTokenExchangeFlow (0.00s)

=== RUN   TestTokenExchangeServiceAccountNotFound
--- PASS: TestTokenExchangeServiceAccountNotFound (0.00s)

=== RUN   TestTokenValidationFails
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

1. **Realistic**: Uses actual Kubernetes client-go fake clients
2. **Standard**: Follows Kubernetes testing conventions
3. **Fast**: No external dependencies or cluster access needed
4. **Verifiable**: Human can see exactly what was tested
5. **Maintainable**: Uses same structs and types as production code
6. **Comprehensive**: Tests both happy path and error cases

## Dependencies

No new dependencies needed - all required for testing:
- `k8s.io/client-go/kubernetes/fake` (already in go.mod)
- `k8s.io/client-go/testing` (already in go.mod)
- `k8s.io/api/core/v1` (already in go.mod)
- `k8s.io/api/authentication/v1` (already in go.mod)

## Future Enhancements

1. Add benchmark tests for performance verification
2. Add table-driven tests for multiple service account combinations
3. Add tests for token caching (when implemented)
4. Add E2E tests with real JWT token generation and parsing
5. Add fuzz tests for malformed input handling

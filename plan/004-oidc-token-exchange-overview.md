# Plan 004: OIDC Token Exchange for Envoy External Authorization

**Status**: Approved

## Overview

Implement an Envoy external authorizer (ext_authz) that exchanges OIDC ID tokens for Kubernetes service accounts in one cluster for valid ID tokens for Kubernetes service accounts in another cluster.

## Primary Use Case: External Secrets Operator with System Gateway

### Architecture

**Workload Cluster** → **System Gateway (Management Cluster)** → **Management Cluster API Server**

```
┌────────────────────────────────────┐
│     Workload Kubernetes Cluster    │
│                                    │
│  ┌──────────────────────────────┐  │
│  │ External Secrets Operator    │  │
│  │ (ESO)                        │  │
│  │                              │  │
│  │ SecretStore:                 │  │
│  │   namespace: app-prod        │  │
│  │   serviceAccount: eso-sa     │  │
│  └──────────────────────────────┘  │
│              │                     │
│              │ Bearer Token        │
│              │ (workload SA)       │
│              ▼                     │
└────────────────────────────────────┘
               │
               │ HTTPS with Bearer Token
               │ Authorization: Bearer <workload-token>
               ▼
┌────────────────────────────────────┐
│  Management Kubernetes Cluster     │
│                                    │
│  ┌──────────────────────────────┐  │
│  │ System Gateway (Istio)       │  │
│  │                              │  │
│  │ AuthorizationPolicy (CUSTOM) │  │
│  │   ↓                          │  │
│  │ Tokensmith (ext_authz)       │  │
│  │   1. Validate workload token │  │
│  │   2. Exchange for mgmt token │  │
│  │      (same namespace/name)   │  │
│  │   3. Forward with new token  │  │
│  └──────────────────────────────┘  │
│              │                     │
│              │ New Bearer Token    │
│              │ (management SA)     │
│              ▼                     │
│  ┌──────────────────────────────┐  │
│  │ Kubernetes API Server        │  │
│  │                              │  │
│  │ GET /api/v1/namespaces/      │  │
│  │     app-prod/secrets/...     │  │
│  └──────────────────────────────┘  │
└────────────────────────────────────┘
```

### Workflow

1. **ESO in Workload Cluster** makes request to system gateway with its service account token
   - ServiceAccount: `app-prod/eso-sa` (workload cluster)
   - Token: Signed by workload cluster OIDC issuer

2. **Istio Gateway** intercepts request, calls Tokensmith (ext_authz) with CUSTOM AuthorizationPolicy

3. **Tokensmith validates** workload token:
   - Verifies signature using workload cluster JWKS
   - Validates standard OIDC claims (iss, aud, exp, iat)
   - Extracts service account identity from `sub` claim
   - Parses namespace and service account name

4. **Tokensmith exchanges** token:
   - Maps to identical service account in management cluster: `app-prod/eso-sa`
   - Uses `client-go` TokenRequest API to create token for management cluster SA
   - Returns new token in modified Authorization header

5. **Request forwarded** to management cluster API server with new token

6. **API server authorizes** using management cluster RBAC for `app-prod/eso-sa`

7. **ESO receives secret** data from management cluster

## Goals

1. Accept incoming requests from Envoy with workload cluster service account tokens
2. Validate the incoming token using Kubernetes TokenReview API
3. Exchange for management cluster token using TokenRequest API (identical namespace/name)
4. Return the new token to Envoy for use in downstream API server requests

## Components

### 1. Envoy External Authorization Server
- Implements the Envoy ext_authz gRPC API
- Receives authorization check requests from Istio gateway
- Returns authorization decisions with modified Authorization header
- Integrates with Istio's CUSTOM AuthorizationPolicy

### 2. Token Validator (Workload Cluster)
Uses **Kubernetes TokenReview API** via `client-go`:

**API**: `authentication.k8s.io/v1.TokenReview`

```go
// TokenReview validates the token against workload cluster
tokenReview := &authenticationv1.TokenReview{
    Spec: authenticationv1.TokenReviewSpec{
        Token: bearerToken, // From Authorization header
    },
}

// Call workload cluster API server
result, err := workloadClient.AuthenticationV1().
    TokenReviews().Create(ctx, tokenReview, metav1.CreateOptions{})

// Extract identity from result.Status
// - result.Status.Authenticated (bool)
// - result.Status.User.Username (e.g., "system:serviceaccount:app-prod:eso-sa")
// - result.Status.User.UID
// - result.Status.Audiences
```

**What TokenReview validates**:
- Token signature (via API server's OIDC validation)
- Token expiration
- Token revocation status
- Service account existence
- Returns service account identity

### 3. Token Exchanger (Management Cluster)
Uses **Kubernetes TokenRequest API** via `client-go`:

**API**: `authentication.k8s.io/v1.TokenRequest` (subresource of ServiceAccount)

```go
// Parse namespace and name from validated identity
// "system:serviceaccount:app-prod:eso-sa" -> namespace="app-prod", name="eso-sa"

// TokenRequest creates a new token for management cluster SA
tokenRequest := &authenticationv1.TokenRequest{
    Spec: authenticationv1.TokenRequestSpec{
        Audiences: []string{"https://kubernetes.default.svc"},
        // Optional: ExpirationSeconds (default 1 hour)
    },
}

// Call management cluster API server
token, err := mgmtClient.CoreV1().
    ServiceAccounts(namespace).
    CreateToken(ctx, name, tokenRequest, metav1.CreateOptions{})

// Returns: token.Status.Token (JWT bearer token)
```

**Service Account Mapping**:
- **Identity-based mapping**: Maps to identical namespace/name in management cluster
- Example: `workload:app-prod/eso-sa` → `management:app-prod/eso-sa`
- Service account must exist in management cluster
- RBAC policies control what the SA can access

### 4. Kubernetes Client Configuration
Two `client-go` clients required:

**Workload Cluster Client**:
- Configured with kubeconfig or in-cluster config for workload cluster
- Used for TokenReview validation
- Requires minimal permissions (just to call TokenReview API)

**Management Cluster Client**:
- Configured with in-cluster config (Tokensmith runs in management cluster)
- Used for TokenRequest creation
- Requires permissions to create tokens for service accounts:
  ```yaml
  apiVersion: rbac.authorization.k8s.io/v1
  kind: ClusterRole
  rules:
  - apiGroups: [""]
    resources: ["serviceaccounts/token"]
    verbs: ["create"]
  ```

## Implementation Steps

### Phase 1: Kubernetes Client Setup

1. Add `client-go` dependency and configuration
   - Add `k8s.io/client-go` to go.mod
   - Add `k8s.io/api` for API types (authenticationv1, etc.)
   - Create configuration loader for dual cluster setup

2. Initialize Kubernetes clients
   - **Workload cluster client**: Load from kubeconfig or service account
   - **Management cluster client**: Use in-cluster config
   - Add health checks to verify cluster connectivity

3. Create client wrapper interfaces
   - Abstract TokenReview operations (for testing)
   - Abstract TokenRequest operations (for testing)
   - Support client rotation/refresh

### Phase 2: Envoy ext_authz Server

1. Implement Envoy ext_authz gRPC server
   - Use Envoy ext_authz protobuf definitions from go-control-plane
   - Implement Check() RPC method
   - Extract bearer token from Authorization header
   - Return OK with modified Authorization header or DENIED

2. Add configuration loading
   - Workload cluster API server endpoint
   - Management cluster in-cluster config
   - Service account mapping rules (if needed beyond identity-based)
   - YAML configuration file support

### Phase 3: Token Validation (Workload Cluster)

1. Implement TokenReview validator using `client-go`
   - Extract bearer token from `Authorization: Bearer <token>` header
   - Create TokenReview request
   - Call workload cluster API: `AuthenticationV1().TokenReviews().Create()`
   - Check `result.Status.Authenticated` (must be true)
   - Extract service account from `result.Status.User.Username`
   - Parse format: `system:serviceaccount:<namespace>:<name>`

2. Add validation logic
   - Verify token is for a service account (not user)
   - Extract namespace and service account name
   - Validate username format matches expected pattern
   - Add error handling for API failures

3. Add error handling and logging
   - Detailed error messages for debugging
   - Structured logging for verification attempts
   - Log TokenReview API call metrics

### Phase 4: Token Exchange (Management Cluster)

1. Implement TokenRequest exchanger using `client-go`
   - Parse namespace and name from validated workload identity
   - Verify management cluster has matching service account
   - Create TokenRequest for management cluster SA
   - Call management cluster API: `CoreV1().ServiceAccounts(ns).CreateToken()`
   - Extract new token from `token.Status.Token`

2. Implement service account mapping
   - **Identity-based mapping** (default): Use same namespace/name
   - Verify service account exists before creating token
   - Handle errors gracefully (SA not found, insufficient permissions)

3. Add security controls
   - Rate limiting per service account
   - Audit logging of all token exchanges
   - Configurable token expiration (default 1 hour)

### Phase 4: Integration and Testing

1. Add comprehensive unit tests
   - Token validation tests
   - Token generation tests
   - Mapping logic tests

2. Add integration tests
   - End-to-end flow testing
   - Envoy integration testing

3. Add documentation
   - Configuration examples
   - Deployment guide
   - Troubleshooting guide

## Configuration Example

```yaml
server:
  address: ":9001"

source:
  issuer: "https://kubernetes.default.svc.cluster.local"
  jwks_uri: "https://kubernetes.default.svc.cluster.local/openid/v1/jwks"

target:
  issuer: "https://target-cluster.example.com"
  signing_key_path: "/etc/tokensmith/signing-key.pem"

mappings:
  # Direct mapping
  - source: "system:serviceaccount:namespace1:service1"
    target: "system:serviceaccount:namespace2:service1"

  # Pattern-based mapping
  - source_pattern: "system:serviceaccount:prod-.*:(.*)"
    target_pattern: "system:serviceaccount:staging-$1:$1"
```

## Security Considerations

1. **Token Validation**: Strictly validate all incoming tokens
2. **Mapping Security**: Default deny; explicit allow only
3. **Key Management**: Secure storage and rotation of signing keys
4. **Logging**: Audit log all token exchanges
5. **Rate Limiting**: Prevent abuse and DoS
6. **TLS**: Require mTLS for Envoy communication

## Dependencies

### Core Libraries

- `google.golang.org/grpc` - gRPC server implementation
- `github.com/envoyproxy/go-control-plane` - Envoy ext_authz API definitions
- `github.com/coreos/go-oidc/v3` - OIDC token verification with JWKS caching
- `github.com/golang-jwt/jwt/v5` - JWT creation and signing
- `gopkg.in/yaml.v3` - Configuration parsing

### OIDC Token Verification: `coreos/go-oidc/v3`

**Chosen for robust JWKS handling and concurrency safety.**

**Key Features:**
- **Smart Key Refresh**: Automatically retries verification after fetching fresh JWKS on signature failure
  1. First attempt: Verify with cached keys
  2. On failure: Fetch new keys from JWKS endpoint
  3. Second attempt: Retry verification with fresh keys
  4. Only fail if both attempts fail

- **Concurrency-Safe**: Uses "inflight" pattern to prevent duplicate concurrent JWKS fetches
  - Multiple goroutines wait on same in-flight request
  - Thread-safe cache updates

- **Production-Ready**:
  - Most widely adopted Go OIDC library
  - Used by Kubernetes and major projects
  - Mature, well-tested implementation

**Why not `zitadel/oidc`?**
While ZITADEL maintains their own certified OIDC library, they use `coreos/go-oidc` for token verification. Their library is excellent for full OIDC server/client implementations but `coreos/go-oidc` is lighter weight and specifically optimized for token verification, which is our primary use case.

**Usage Pattern:**
```go
import "github.com/coreos/go-oidc/v3/oidc"

// Create provider (with OIDC discovery)
provider, err := oidc.NewProvider(ctx, issuerURL)

// Create verifier with configuration
verifier := provider.Verifier(&oidc.Config{
    ClientID: "audience",
})

// Verify token (handles JWKS caching and refresh automatically)
idToken, err := verifier.Verify(ctx, rawToken)
```

## Success Criteria

1. Successfully validates OIDC ID tokens from source cluster
2. Generates valid ID tokens for target cluster
3. Integrates with Envoy via ext_authz API
4. Handles errors gracefully with appropriate responses
5. Provides comprehensive logging and monitoring
6. Includes complete test coverage (>80%)

## Future Enhancements

1. Support for token caching to reduce load
2. Support for multiple source/target cluster pairs
3. Metrics and observability (Prometheus)
4. Dynamic configuration reload
5. Support for other token types (OAuth2 access tokens)

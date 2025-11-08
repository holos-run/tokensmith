# Plan 001: OIDC Token Exchange for Envoy External Authorization

## Overview

Implement an Envoy external authorizer (ext_authz) that exchanges OIDC ID tokens for Kubernetes service accounts in one cluster for valid ID tokens for Kubernetes service accounts in another cluster.

## Goals

1. Accept incoming requests from Envoy with OIDC ID tokens
2. Validate the incoming ID token from the source cluster
3. Exchange the validated token for a new ID token valid in the target cluster
4. Return the new token to Envoy for use in downstream requests

## Architecture

### Components

1. **Envoy External Authorization Server**
   - Implements the Envoy ext_authz gRPC API
   - Receives authorization check requests from Envoy
   - Returns authorization decisions with modified headers

2. **Token Validator**
   - Validates incoming OIDC ID tokens
   - Verifies token signature using JWKS from source cluster
   - Validates claims (issuer, audience, expiration, etc.)

3. **Token Exchanger**
   - Creates new ID tokens for target cluster service accounts
   - Signs tokens using target cluster credentials
   - Maps source service account to target service account

4. **Configuration Management**
   - Source cluster OIDC configuration (issuer, JWKS endpoint)
   - Target cluster configuration (issuer, signing keys)
   - Service account mapping rules

## Implementation Steps

### Phase 1: Basic Server Setup

1. Implement Envoy ext_authz gRPC server
   - Define protobuf service definition
   - Implement Check() RPC method
   - Add basic request/response handling

2. Add configuration loading
   - YAML configuration file support
   - Environment variable overrides
   - Validation of configuration

### Phase 2: Token Validation

1. Implement OIDC token validator
   - Fetch and cache JWKS from source cluster
   - Validate token signature
   - Validate standard claims (iss, aud, exp, iat, nbf)
   - Validate Kubernetes-specific claims (sub format)

2. Add error handling and logging
   - Detailed error messages for debugging
   - Structured logging

### Phase 3: Token Exchange

1. Implement token generation
   - Create new JWT with target cluster claims
   - Sign with target cluster private key
   - Include appropriate claims (iss, aud, sub, exp, iat)

2. Implement service account mapping
   - Define mapping configuration format
   - Support 1:1 and pattern-based mappings
   - Default deny if no mapping found

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

- `google.golang.org/grpc` - gRPC server implementation
- `github.com/envoyproxy/go-control-plane` - Envoy ext_authz API definitions
- `github.com/golang-jwt/jwt/v5` - JWT creation and validation
- `gopkg.in/yaml.v3` - Configuration parsing

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

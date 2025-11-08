# Plan 006: Multi-Cluster Token Authorization Configuration

**Status**: ✅ Complete
**Date**: 2025-11-07

## Overview

Implemented YAML-based configuration for validating tokens from multiple workload clusters using JWKS (JSON Web Key Set), eliminating the need for TokenReview API calls to save network traffic.

## Objectives

1. Support validation of tokens from multiple workload clusters
2. Avoid network calls to workload clusters using JWKS-based validation
3. Map namespaces from workload clusters to the same namespace in the management cluster
4. Provide documentation for extracting and configuring JWKS from clusters
5. Maintain backward compatibility with existing TokenReview-based validation

## Implementation

### 1. Configuration Structures

**File**: [internal/config/clusters.go](../internal/config/clusters.go)

Created configuration types:
- `ClustersConfig`: Contains array of cluster configurations
- `ClusterConfig`: Defines single cluster with:
  - `Name`: Human-readable cluster identifier
  - `Issuer`: OIDC issuer URL (must match JWT `iss` claim)
  - `JWKSURI`: Optional URL to fetch JWKS from
  - `JWKSData`: Optional inline JWKS data (recommended to avoid runtime network calls)

Validation ensures:
- At least one cluster is configured
- No duplicate issuers or names
- Either `JWKSURI` or `JWKSData` is provided
- JWKSData contains at least one key if provided

### 2. Configuration Loader

**File**: [internal/config/loader.go](../internal/config/loader.go)

Implemented `LoadClustersConfig` function that:
- Reads YAML file from specified path
- Parses configuration using `gopkg.in/yaml.v3`
- Validates configuration structure
- Returns parsed and validated configuration

### 3. JWKS Validation Logic

**File**: [internal/token/jwks.go](../internal/token/jwks.go)

Implemented `JWKSValidator` that:
- Validates JWT tokens using `go-jose/go-jose/v4` library
- Matches token issuer to configured cluster
- Verifies JWT signature using cluster's JWKS
- Validates standard JWT claims (exp, nbf, iat)
- Extracts Kubernetes service account identity from claims
- Caches fetched JWKS for 1 hour (when using JWKSURI)
- Returns same `ServiceAccountIdentity` struct as TokenReview validator

Key features:
- Supports both inline JWKS data and JWKS URI fetching
- Handles multiple keys per cluster (for key rotation scenarios)
- Validates flattened Kubernetes claims format:
  - `kubernetes.io/serviceaccount/namespace`
  - `kubernetes.io/serviceaccount/service-account.name`
  - `kubernetes.io/serviceaccount/service-account.uid`

### 4. Updated Token Validator

**File**: [internal/token/validator.go](../internal/token/validator.go)

Changes:
- Introduced `TokenValidator` interface for both validation methods
- Kept existing `Validator` (TokenReview-based) as implementation
- `JWKSValidator` implements same interface
- No breaking changes to existing code

### 5. Command-Line Interface Updates

**File**: [cmd/tokensmith/commands/authz.go](../cmd/tokensmith/commands/authz.go)

Added support for both validation modes:
- New `--clusters-config` flag for YAML configuration file path
- Deprecated `--workload-kubeconfig` (still supported for backward compatibility)
- Logic to choose between JWKS and TokenReview validation based on flags
- When using `--clusters-config`:
  - Only management cluster client is initialized
  - Validation uses JWKS from configuration
  - No network calls to workload clusters

### 6. Documentation

**File**: [docs/cluster-config-setup.md](../docs/cluster-config-setup.md)

Comprehensive guide covering:
- Configuration file format and fields
- Step-by-step instructions to extract JWKS from Kubernetes clusters
- How to deploy configuration using ConfigMaps
- Token validation flow diagram
- Issuer matching requirements
- Security considerations (JWKS trust, key rotation)
- Troubleshooting guide
- Migration path from TokenReview mode

### 7. Integration Tests

**File**: [internal/token/jwks_test.go](../internal/token/jwks_test.go)

Test coverage:
- ✅ Validate tokens from multiple clusters
- ✅ Match tokens to correct cluster by issuer
- ✅ Reject tokens from unknown issuers
- ✅ Reject expired tokens
- ✅ Reject tokens with invalid signatures
- ✅ Reject malformed tokens
- ✅ Support multiple keys per cluster (key rotation)
- ✅ Extract service account identity from various claim formats

**File**: [internal/testutil/jwt.go](../internal/testutil/jwt.go) (updated)

Added:
- Key ID (kid) generation and inclusion in JWT headers
- `GenerateTokenFlatClaims` method for real Kubernetes token format
- `KeyID()` method to expose key ID for JWKS creation
- Maintained backward compatibility with `GenerateToken` for existing tests

### 8. Dependencies

**File**: [go.mod](../go.mod)

Added dependencies:
- `github.com/go-jose/go-jose/v4 v4.1.3` - JWT/JWKS handling
- `gopkg.in/yaml.v3` - YAML configuration parsing

## Configuration Example

```yaml
clusters:
  - name: workload-cluster-1
    issuer: https://kubernetes.default.svc.cluster.local
    jwks_data:
      keys:
        - kty: RSA
          use: sig
          kid: abc123
          n: ...base64-encoded-modulus...
          e: AQAB

  - name: workload-cluster-2
    issuer: https://10.96.0.1
    jwks_data:
      keys:
        - kty: RSA
          use: sig
          kid: xyz789
          n: ...base64-encoded-modulus...
          e: AQAB
```

## Usage

### Legacy Mode (TokenReview)

```bash
tokensmith authz \
  --workload-kubeconfig=/path/to/workload.kubeconfig \
  --addr=0.0.0.0 \
  --port=9001
```

### New Mode (JWKS-based Multi-Cluster)

```bash
tokensmith authz \
  --clusters-config=/path/to/clusters.yaml \
  --addr=0.0.0.0 \
  --port=9001
```

## Token Validation Flow

1. **Extract Token**: Get bearer token from Authorization header
2. **Parse JWT**: Extract issuer claim without verification
3. **Find Cluster**: Match issuer to configured cluster
4. **Get JWKS**: Use inline data or fetch from URI (with caching)
5. **Verify Signature**: Validate JWT signature using JWKS
6. **Validate Claims**: Check expiration, not-before, issued-at times
7. **Extract Identity**: Get namespace, service account name, UID from claims
8. **Exchange Token**: Use identity to request management cluster token
9. **Return Response**: Send OK with modified Authorization header

## Benefits

### Performance
- ✅ **No network calls to workload clusters** during validation
- ✅ **Faster token validation** using local JWKS data
- ✅ **JWKS caching** reduces repeated fetches (when using URI)

### Scalability
- ✅ **Support unlimited workload clusters** without connection overhead
- ✅ **Independent cluster operations** - validation doesn't require cluster connectivity
- ✅ **Simple horizontal scaling** - no client connection pooling needed

### Operational
- ✅ **Reduced network dependencies** - fewer points of failure
- ✅ **Simplified networking** - no need to expose workload cluster APIs
- ✅ **Standard OIDC approach** - follows Kubernetes native token format

### Compatibility
- ✅ **Backward compatible** - existing TokenReview mode still works
- ✅ **Zero breaking changes** - same interfaces and data structures
- ✅ **Gradual migration** - can switch per-deployment

## Security Considerations

1. **JWKS Trust**: Only configure JWKS from trusted cluster API servers
2. **Key Rotation**: Include all active keys to support rotation periods
3. **Regular Updates**: Monitor and update JWKS when clusters rotate keys
4. **Namespace Isolation**: Identity mapping preserves namespace boundaries
5. **Issuer Validation**: Strict issuer matching prevents token misuse

## Testing

All tests passing:
```
✅ TestTokenExchangeFlow
✅ TestTokenExchangeServiceAccountNotFound
✅ TestTokenValidationFails
✅ TestTokenExpirationPreserved
✅ TestJWKSValidator_Validate (6 subtests)
✅ TestJWKSValidator_MultipleKeys (2 subtests)
✅ TestExtractServiceAccountIdentity (5 subtests)
✅ TestParseServiceAccountIdentity (7 subtests)
```

Total: 23 tests, all passing

## Files Modified/Created

### Created
- [internal/config/clusters.go](../internal/config/clusters.go) - Configuration structures
- [internal/config/loader.go](../internal/config/loader.go) - YAML loader
- [internal/token/jwks.go](../internal/token/jwks.go) - JWKS validator
- [internal/token/jwks_test.go](../internal/token/jwks_test.go) - Integration tests
- [docs/cluster-config-setup.md](../docs/cluster-config-setup.md) - User documentation

### Modified
- [internal/token/validator.go](../internal/token/validator.go) - Added TokenValidator interface
- [internal/authz/extauthz.go](../internal/authz/extauthz.go) - Use TokenValidator interface
- [cmd/tokensmith/commands/authz.go](../cmd/tokensmith/commands/authz.go) - Support both modes
- [internal/testutil/jwt.go](../internal/testutil/jwt.go) - Added flat claims generation
- [go.mod](../go.mod) - Added dependencies
- [go.sum](../go.sum) - Updated checksums

## Future Enhancements

Potential improvements for future iterations:

1. **Automatic JWKS Refresh**: Periodically refresh JWKS from URI
2. **Configuration Hot Reload**: Update clusters config without restart
3. **Metrics**: Track validation success/failure per cluster
4. **OIDC Discovery**: Auto-fetch JWKS from `.well-known/openid-configuration`
5. **Cluster Health**: Report validation stats per configured cluster

## Conclusion

Successfully implemented multi-cluster token authorization with JWKS-based validation. The system now supports:

- Multiple workload clusters without network dependencies
- Standard OIDC/JWKS validation approach
- Backward compatibility with TokenReview mode
- Comprehensive documentation and testing
- Simple YAML-based configuration

The implementation maintains the existing token exchange behavior while adding the ability to scale to many workload clusters efficiently.

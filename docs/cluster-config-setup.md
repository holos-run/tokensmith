# Multi-Cluster Configuration Setup

This guide explains how to configure TokenSmith for multi-cluster token authorization using JWKS-based validation.

## Overview

TokenSmith can validate tokens from multiple workload clusters without making network calls to those clusters. Instead, it validates tokens using the public JWKS (JSON Web Key Set) from each cluster's OIDC issuer.

This approach has several advantages:
- **No network dependency**: Validation doesn't require connectivity to workload clusters
- **Better performance**: No TokenReview API calls to remote clusters
- **Scalability**: Support for many workload clusters without creating connections to each

## Configuration File Format

Create a YAML configuration file that defines your workload clusters:

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
    jwks_uri: https://workload-cluster-2.example.com/openid/v1/jwks
```

### Configuration Fields

#### Cluster Configuration

- **name** (required): Human-readable identifier for the cluster
- **issuer** (required): OIDC issuer URL that must match the `iss` claim in tokens
- **jwks_data** (optional): Inline JWKS data containing public keys
- **jwks_uri** (optional): URL to fetch JWKS from

**Note**: Either `jwks_data` or `jwks_uri` must be provided. Using `jwks_data` is recommended to avoid runtime network calls.

## Extracting JWKS from a Kubernetes Cluster

### Step 1: Fetch OpenID Configuration

First, retrieve the OpenID configuration from your workload cluster to identify the issuer and JWKS endpoint:

```bash
kubectl get --raw /.well-known/openid-configuration | jq .
```

This will return JSON like:

```json
{
  "issuer": "https://kubernetes.default.svc.cluster.local",
  "jwks_uri": "https://kubernetes.default.svc.cluster.local/openid/v1/jwks",
  "response_types_supported": ["id_token"],
  "subject_types_supported": ["public"],
  "id_token_signing_alg_values_supported": ["RS256"]
}
```

Note the `issuer` and `jwks_uri` values.

### Step 2: Fetch JWKS Data

Retrieve the public keys from the JWKS endpoint:

```bash
kubectl get --raw /openid/v1/jwks | jq .
```

This will return the JWKS:

```json
{
  "keys": [
    {
      "use": "sig",
      "kty": "RSA",
      "kid": "zSLNp7RbfFKCN7zNdD8fZKvQBkLKbGq7L9Yg4Xx2YsI",
      "alg": "RS256",
      "n": "xGOr-H7A...",
      "e": "AQAB"
    }
  ]
}
```

### Step 3: Create Configuration File

Create a YAML file (`clusters-config.yaml`) with the extracted data:

```yaml
clusters:
  - name: my-workload-cluster
    issuer: https://kubernetes.default.svc.cluster.local
    jwks_data:
      keys:
        - use: sig
          kty: RSA
          kid: zSLNp7RbfFKCN7zNdD8fZKvQBkLKbGq7L9Yg4Xx2YsI
          alg: RS256
          n: xGOr-H7A...
          e: AQAB
```

**Important**: Copy the entire JWKS structure, including all keys. Kubernetes may rotate between multiple signing keys.

## Deploying with ConfigMap

When deploying TokenSmith in Kubernetes, store the configuration in a ConfigMap:

### Create ConfigMap

```bash
kubectl create configmap tokensmith-clusters-config \
  --from-file=clusters.yaml=clusters-config.yaml \
  -n tokensmith-system
```

### Mount ConfigMap in Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tokensmith
  namespace: tokensmith-system
spec:
  template:
    spec:
      containers:
      - name: tokensmith
        image: tokensmith:latest
        args:
          - authz
          - --clusters-config=/config/clusters.yaml
          - --addr=0.0.0.0
          - --port=9001
        volumeMounts:
          - name: clusters-config
            mountPath: /config
            readOnly: true
      volumes:
        - name: clusters-config
          configMap:
            name: tokensmith-clusters-config
```

## Running TokenSmith

Start the authorization server with the clusters configuration:

```bash
tokensmith authz --clusters-config=clusters-config.yaml
```

The server will:
1. Load and validate the clusters configuration
2. Initialize JWKS validators for each cluster
3. Start the ext_authz gRPC server

When a token is presented:
1. Parse the JWT to extract the `iss` (issuer) claim
2. Match the issuer to a configured cluster
3. Validate the token signature using that cluster's JWKS
4. Extract the service account identity from the token claims
5. Exchange it for a management cluster token

## Token Validation Flow

```
┌─────────────────┐
│  Workload Pod   │
│  (SA Token)     │
└────────┬────────┘
         │
         │ 1. Request with Bearer token
         ▼
┌─────────────────────────┐
│   Istio Envoy Proxy     │
│   (ext_authz client)    │
└────────┬────────────────┘
         │
         │ 2. ext_authz Check(token)
         ▼
┌─────────────────────────┐
│   TokenSmith Server     │
│                         │
│  ┌─────────────────┐   │
│  │ Parse JWT       │   │ 3. Extract issuer from token
│  └────────┬────────┘   │
│           │             │
│  ┌────────▼────────┐   │
│  │ Find Cluster    │   │ 4. Match issuer to cluster config
│  └────────┬────────┘   │
│           │             │
│  ┌────────▼────────┐   │
│  │ Validate JWKS   │   │ 5. Verify signature with cluster's JWKS
│  └────────┬────────┘   │
│           │             │
│  ┌────────▼────────┐   │
│  │ Extract SA ID   │   │ 6. Get namespace/name from claims
│  └────────┬────────┘   │
│           │             │
│  ┌────────▼────────┐   │
│  │ Token Exchange  │   │ 7. Get mgmt cluster token
│  └────────┬────────┘   │
└───────────┼─────────────┘
            │
            │ 8. OK + new token
            ▼
┌─────────────────────────┐
│   Istio Envoy Proxy     │
│   (forwards to backend) │
└─────────────────────────┘
```

## Issuer Matching

The `issuer` field in the configuration must **exactly match** the `iss` claim in tokens from that cluster. Common issuer values include:

- `https://kubernetes.default.svc.cluster.local` - Default for many clusters
- `https://kubernetes.default.svc` - Alternative default
- `https://10.96.0.1` - Using cluster IP directly
- Custom issuers configured via `--service-account-issuer` flag on kube-apiserver

To find your cluster's issuer, decode a service account token or check the OpenID configuration as shown above.

## Security Considerations

### JWKS Trust

The JWKS data contains the **public keys** used to verify token signatures. These keys should be treated as sensitive because:

1. They define which tokens are trusted by your system
2. An incorrect JWKS could allow unauthorized access
3. JWKS should only be fetched from trusted cluster API servers

### Key Rotation

Kubernetes periodically rotates service account signing keys. When this happens:

1. The old key remains valid for existing tokens until they expire
2. New tokens are signed with the new key
3. Both keys appear in the JWKS

**Recommendation**: Include all keys from the JWKS in your configuration, and update the ConfigMap when keys are rotated.

### Monitoring Key Rotation

To detect when keys need updating:

1. Watch for token validation failures with unknown `kid` (key ID)
2. Periodically compare your configured JWKS with the live cluster's JWKS
3. Consider implementing automated JWKS refresh if using `jwks_uri`

### Namespace Isolation

TokenSmith preserves the namespace isolation from the workload cluster:

- Tokens for `namespace-a/service-account-x` can only exchange to the same namespace and service account in the management cluster
- The management cluster must have the corresponding service account already created
- There is no cross-namespace token exchange

## Troubleshooting

### Token Validation Fails

**Error**: "unknown issuer"
- **Cause**: Token's `iss` claim doesn't match any configured cluster
- **Fix**: Check the token's issuer (decode the JWT) and ensure it's in your config

**Error**: "failed to verify token"
- **Cause**: Token signature verification failed
- **Fix**: Ensure JWKS contains the key with matching `kid` from the token header

### Service Account Not Found

**Error**: "service account not found in management cluster"
- **Cause**: The management cluster doesn't have the service account
- **Fix**: Create the service account in the management cluster:
  ```bash
  kubectl create serviceaccount <name> -n <namespace>
  ```

### Configuration Validation Errors

**Error**: "either jwks_uri or jwks_data must be provided"
- **Fix**: Add `jwks_data` or `jwks_uri` to each cluster configuration

**Error**: "duplicate issuer"
- **Fix**: Each cluster must have a unique issuer

## Example: Three Workload Clusters

```yaml
clusters:
  # Production East
  - name: prod-east
    issuer: https://prod-east.k8s.example.com
    jwks_data:
      keys:
        - kty: RSA
          use: sig
          kid: prod-east-key-1
          n: ...
          e: AQAB

  # Production West
  - name: prod-west
    issuer: https://prod-west.k8s.example.com
    jwks_data:
      keys:
        - kty: RSA
          use: sig
          kid: prod-west-key-1
          n: ...
          e: AQAB

  # Development
  - name: dev
    issuer: https://kubernetes.default.svc
    jwks_data:
      keys:
        - kty: RSA
          use: sig
          kid: dev-key-1
          n: ...
          e: AQAB
```

Each workload cluster's pods can present their tokens, and TokenSmith will:
1. Identify which cluster the token came from based on the issuer
2. Validate using that cluster's JWKS
3. Exchange for a management cluster token with the same identity

## Migration from TokenReview Mode

If you're currently using `--workload-kubeconfig`, you can migrate to the new multi-cluster mode:

1. Extract JWKS from your workload cluster (see above)
2. Create a clusters configuration file with one cluster
3. Test with `--clusters-config` flag
4. Once verified, remove `--workload-kubeconfig` flag
5. Delete workload cluster kubeconfig from deployment (no longer needed)

The new mode is backward compatible - you can continue using `--workload-kubeconfig` if you prefer TokenReview-based validation.

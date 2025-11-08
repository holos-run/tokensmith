# Plan 007: Token Cache with Background Garbage Collection

**Status**: 📋 Planned
**Date**: 2025-11-07

## Overview

Implement a simple token cache to avoid creating duplicate management cluster tokens when the same workload token is presented repeatedly (e.g., External Secrets Operator polling every 30 seconds).

## Problem Statement

Currently, every call to the `Exchange()` method creates a **new token on the management cluster** via the Kubernetes TokenRequest API, even when the same workload identity is making repeated requests. This results in:

- Excessive API calls to the management cluster
- Unnecessary token creation overhead
- Performance impact for high-frequency token exchange scenarios (ESO polling, health checks, etc.)

**Current Flow** (from [internal/token/exchanger.go:71-76](../internal/token/exchanger.go#L71-L76)):
```
Same Workload Token → Validate → Exchange → CreateToken API → New Management Token
Same Workload Token → Validate → Exchange → CreateToken API → New Management Token
Same Workload Token → Validate → Exchange → CreateToken API → New Management Token
```

Each exchange currently triggers:
1. ServiceAccount GET from management cluster
2. CreateToken API call to management cluster
3. New JWT with unique `jti` (JWT ID) and `iat` (issued at) timestamp

## Solution: Simple Token Cache

Implement an in-memory cache that stores management cluster tokens indexed by the **workload service account UID** with periodic garbage collection.

### Cache Design

**Cache Key**: Workload service account UID (from workload cluster)
- Extracted from `identity.UID` (type `types.UID`)
- Example: `"72b0e9c5-c44a-4de0-ae59-9b400f1221e0"`
- Ensures **1:1 mapping** between workload identity and cached management token
- **UUID uniqueness guarantees no collisions** between different service accounts

**Cache Value**:
```go
type cacheEntry struct {
    token     string
    expiresAt time.Time
}
```

**Cache TTL**: Use token expiration time (typically 1 hour)

**Garbage Collection**: Background goroutine runs every 5 minutes

### Why Index by Workload UID?

The workload service account UID is the perfect cache key because:

1. **Unique per service account**: Each SA in a cluster has a unique UUID
2. **Stable**: UID doesn't change for the lifetime of the service account
3. **Already extracted**: Available from `identity.UID` after validation
4. **No collision risk**: UUIDs are globally unique
5. **Simple 1:1 mapping**: One workload identity → one cached management token

This is simpler than composite keys like `(namespace, name, audience, expiration)` and provides the same cache effectiveness.

## Implementation Steps

### 1. Create Cache Package

**File**: `internal/token/cache.go`

Implement a thread-safe cache with:

```go
type Cache struct {
    mu      sync.RWMutex
    entries map[string]cacheEntry
    stopCh  chan struct{}
}

type cacheEntry struct {
    token     string
    expiresAt time.Time
}

// Public API
func NewCache() *Cache
func (c *Cache) Get(uid string) (token string, found bool)
func (c *Cache) Set(uid string, token string, expiresAt time.Time)
func (c *Cache) Stop()

// Internal
func (c *Cache) cleanup()
func (c *Cache) start()
```

**Background Goroutine**:
- Starts in `NewCache()`
- Runs every 5 minutes
- Iterates through entries, removes expired ones
- Graceful shutdown via `stopCh`

**Thread Safety**:
- Use `sync.RWMutex` for concurrent access
- Read lock for `Get()` operations
- Write lock for `Set()` and `cleanup()`

### 2. Update Exchanger

**File**: `internal/token/exchanger.go`

Modify the `Exchanger` struct:

```go
type Exchanger struct {
    client kubernetes.Interface
    cache  *Cache
}

func NewExchanger(client kubernetes.Interface) *Exchanger {
    return &Exchanger{
        client: client,
        cache:  NewCache(),
    }
}
```

Modify `Exchange()` method logic:

```go
func (e *Exchanger) Exchange(ctx context.Context, identity *ServiceAccountIdentity, audiences []string, expiresAt time.Time) (string, error) {
    // 1. Try cache first
    cacheKey := string(identity.UID)
    if token, found := e.cache.Get(cacheKey); found {
        return token, nil
    }

    // 2. Cache miss - proceed with existing logic
    sa, err := e.client.CoreV1().ServiceAccounts(identity.Namespace).Get(ctx, identity.Name, metav1.GetOptions{})
    // ... existing code ...

    // 3. Create new token via API
    result, err := e.client.CoreV1().ServiceAccounts(identity.Namespace).CreateToken(...)
    // ... existing code ...

    // 4. Store in cache before returning
    e.cache.Set(cacheKey, result.Status.Token, expiresAt)

    return result.Status.Token, nil
}
```

### 3. Add Unit Tests

**File**: `internal/token/cache_test.go`

Test coverage:

```go
func TestCache_GetSet(t *testing.T)           // Basic get/set operations
func TestCache_Expiration(t *testing.T)       // Expired entries return not found
func TestCache_Cleanup(t *testing.T)          // GC removes expired entries
func TestCache_Concurrent(t *testing.T)       // Thread-safe concurrent access
func TestCache_Stop(t *testing.T)             // Graceful shutdown
```

### 4. Update Integration Tests

**File**: `internal/token/exchange_integration_test.go`

Add test case:

```go
func TestTokenExchangeCache(t *testing.T) {
    // 1. First exchange - cache miss
    token1, err := exchanger.Exchange(ctx, identity, audiences, expiresAt)

    // 2. Second exchange - cache hit (same UID)
    token2, err := exchanger.Exchange(ctx, identity, audiences, expiresAt)

    // 3. Verify same token returned
    assert.Equal(t, token1, token2)

    // 4. Verify no new token created (could mock CreateToken API)
}
```

## Garbage Collection Rationale

**GC Interval**: 5 minutes

**Why 5 minutes?**
- Tokens typically expire in 1 hour (user's system)
- GC runs 12 times per token lifetime
- Low overhead: ~1ms to scan and remove expired entries
- Balances memory cleanup vs. CPU overhead
- Worst case: expired token sits in memory for 5 extra minutes (negligible)

**Alternative Considered**: Lazy expiration (check on Get)
- **Rejected** because memory would grow unbounded without GC
- Stale entries would accumulate if certain SAs stop making requests

## Performance Benefits

**Expected Improvement**:
- **95%+ reduction** in CreateToken API calls for repeated requests
- **50%+ reduction** in token exchange latency (no API round-trip)
- **Reduced management cluster API load**

**Example Scenario** (ESO polling every 30s):
- **Without cache**: 120 CreateToken API calls per hour
- **With cache**: 1 CreateToken API call per hour (on first request)

## Cache Behavior Examples

### Scenario 1: Same Workload Identity, Repeated Requests

```
Request 1: UID "72b0e9c5-..." → Cache miss → CreateToken API → Cache token
Request 2: UID "72b0e9c5-..." → Cache hit → Return cached token
Request 3: UID "72b0e9c5-..." → Cache hit → Return cached token
...
(59 minutes later)
Request N: UID "72b0e9c5-..." → Cache hit → Return cached token
(61 minutes later - token expired)
Request N+1: UID "72b0e9c5-..." → Cache miss (expired) → CreateToken API → Cache new token
```

### Scenario 2: Different Workload Identities

```
Request 1: UID "72b0e9c5-..." → Cache miss → CreateToken API → Cache token A
Request 2: UID "a1b2c3d4-..." → Cache miss → CreateToken API → Cache token B
Request 3: UID "72b0e9c5-..." → Cache hit → Return token A
Request 4: UID "a1b2c3d4-..." → Cache hit → Return token B
```

### Scenario 3: Garbage Collection

```
Time 0:00 - Token A cached (expires 1:00)
Time 0:05 - GC runs, Token A still valid, kept
Time 0:10 - GC runs, Token A still valid, kept
...
Time 1:00 - Token A expires (but still in memory)
Time 1:05 - GC runs, Token A expired, removed
```

## Security Considerations

### Memory Exposure
- **Risk**: Cached tokens stored in process memory could be exposed via memory dumps
- **Mitigation**: Same risk exists for tokens in-flight; no worse than current state
- **Note**: Tokens are short-lived (1 hour) and bound to specific SA identities

### Cache Poisoning
- **Risk**: None - cache is only populated by our own CreateToken API calls
- **Mitigation**: Not applicable, cache is write-only by exchanger

### Token Revocation
- **Risk**: Cached token remains valid even if source SA is deleted
- **Mitigation**:
  - Tokens expire naturally (1 hour max)
  - Management cluster RBAC still enforces authorization
  - If SA is deleted on management cluster, token becomes unusable regardless of cache
- **Impact**: Low - worst case is 1 hour of access after SA deletion

### Cache Size/DoS
- **Risk**: Unbounded cache could grow with many unique service accounts
- **Mitigation**:
  - GC removes expired entries every 5 minutes
  - Typical deployments have bounded number of SAs making requests
  - Could add max size limit in future if needed (LRU eviction)
- **Estimate**: 1000 SAs × 200 bytes per entry = 200KB memory (negligible)

## Future Enhancements

Potential improvements for future iterations:

1. **Cache Metrics**: Track hit/miss ratio, size, GC duration
2. **Max Size Limit**: LRU eviction if cache exceeds N entries
3. **Configurable GC Interval**: Allow tuning via command-line flag
4. **Cache Warming**: Proactively cache tokens for known SAs
5. **Persistent Cache**: Redis/Memcached for multi-instance deployments

## Testing Strategy

### Unit Tests
- ✅ Cache get/set operations
- ✅ Expiration behavior
- ✅ Garbage collection
- ✅ Concurrent access (race detector)
- ✅ Graceful shutdown

### Integration Tests
- ✅ Cache hit returns same token
- ✅ Cache miss creates new token
- ✅ Expired token triggers new creation
- ✅ Different UIDs get different tokens

### Manual Testing
```bash
# 1. Start tokensmith with debug logging
tokensmith authz --clusters-config=clusters.yaml

# 2. Make repeated requests with same workload token
for i in {1..10}; do
  curl -H "Authorization: Bearer $WORKLOAD_TOKEN" http://localhost:9001/check
done

# 3. Verify logs show:
#    - First request: "cache miss, creating token"
#    - Subsequent requests: "cache hit, returning cached token"

# 4. Wait for token expiration (1 hour) and repeat
# 5. Verify new token created after expiration
```

## Files to Create/Modify

### Created
- `internal/token/cache.go` - Cache implementation
- `internal/token/cache_test.go` - Unit tests

### Modified
- `internal/token/exchanger.go` - Integrate cache into Exchange method
- `internal/token/exchange_integration_test.go` - Add cache behavior tests

## Success Criteria

- ✅ Cache correctly stores and retrieves tokens by workload UID
- ✅ Expired tokens are not returned from cache
- ✅ Garbage collection removes expired entries every 5 minutes
- ✅ Thread-safe for concurrent requests
- ✅ Integration tests verify cache hit/miss behavior
- ✅ No breaking changes to existing API
- ✅ Graceful shutdown stops background goroutine

## Conclusion

This simple token cache will significantly reduce management cluster API load for common scenarios like External Secrets Operator polling, while maintaining security and correctness. The implementation is straightforward, well-tested, and ready for production use.

package token

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_GetSet(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	uid := "72b0e9c5-c44a-4de0-ae59-9b400f1221e0"
	token := "test-token"
	expiresAt := time.Now().Add(1 * time.Hour)

	// Cache should be empty initially
	_, found := cache.Get(uid)
	assert.False(t, found, "cache should be empty initially")

	// Set a token
	cache.Set(uid, token, expiresAt)

	// Get should return the token
	gotToken, found := cache.Get(uid)
	assert.True(t, found, "token should be found")
	assert.Equal(t, token, gotToken, "token should match")
}

func TestCache_Expiration(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	uid := "72b0e9c5-c44a-4de0-ae59-9b400f1221e0"
	token := "test-token"

	// Set token that expires in 100ms
	expiresAt := time.Now().Add(100 * time.Millisecond)
	cache.Set(uid, token, expiresAt)

	// Should be found immediately
	_, found := cache.Get(uid)
	assert.True(t, found, "token should be found before expiration")

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should not be found after expiration
	_, found = cache.Get(uid)
	assert.False(t, found, "token should not be found after expiration")
}

func TestCache_Cleanup(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	// Add some entries with different expiration times
	cache.Set("uid1", "token1", time.Now().Add(-1*time.Hour))  // Already expired
	cache.Set("uid2", "token2", time.Now().Add(1*time.Hour))   // Valid
	cache.Set("uid3", "token3", time.Now().Add(-30*time.Minute)) // Already expired

	// Verify initial state
	cache.mu.RLock()
	assert.Len(t, cache.entries, 3, "should have 3 entries before cleanup")
	cache.mu.RUnlock()

	// Run cleanup
	cache.cleanup()

	// Verify expired entries are removed
	cache.mu.RLock()
	assert.Len(t, cache.entries, 1, "should have 1 entry after cleanup")
	_, found := cache.entries["uid2"]
	assert.True(t, found, "valid entry should remain")
	cache.mu.RUnlock()

	// Verify expired entries are not accessible via Get
	_, found = cache.Get("uid1")
	assert.False(t, found, "expired entry should not be accessible")
	_, found = cache.Get("uid3")
	assert.False(t, found, "expired entry should not be accessible")

	// Verify valid entry is accessible
	token, found := cache.Get("uid2")
	assert.True(t, found, "valid entry should be accessible")
	assert.Equal(t, "token2", token, "token should match")
}

func TestCache_Concurrent(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	// Number of goroutines
	numReaders := 50
	numWriters := 50
	expiresAt := time.Now().Add(1 * time.Hour)

	var wg sync.WaitGroup

	// Start concurrent writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			uid := "uid-" + string(rune(i))
			token := "token-" + string(rune(i))
			cache.Set(uid, token, expiresAt)
		}(i)
	}

	// Start concurrent readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			uid := "uid-" + string(rune(i%numWriters))
			_, _ = cache.Get(uid)
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Verify cache state is consistent
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	assert.LessOrEqual(t, len(cache.entries), numWriters, "should have at most numWriters entries")
}

func TestCache_Stop(t *testing.T) {
	cache := NewCache()

	// Verify goroutine is running by setting a short-lived entry
	cache.Set("uid1", "token1", time.Now().Add(1*time.Hour))

	// Stop the cache
	cache.Stop()

	// Stopping again should not panic
	require.NotPanics(t, func() {
		cache.Stop()
	}, "stopping twice should not panic")

	// Cache should still be readable after stop
	token, found := cache.Get("uid1")
	assert.True(t, found, "cache should still be readable after stop")
	assert.Equal(t, "token1", token, "token should match")
}

func TestCache_MultipleUIDs(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	expiresAt := time.Now().Add(1 * time.Hour)

	// Set tokens for different UIDs
	cache.Set("uid-1", "token-1", expiresAt)
	cache.Set("uid-2", "token-2", expiresAt)
	cache.Set("uid-3", "token-3", expiresAt)

	// Verify each UID gets its own token
	token1, found1 := cache.Get("uid-1")
	assert.True(t, found1)
	assert.Equal(t, "token-1", token1)

	token2, found2 := cache.Get("uid-2")
	assert.True(t, found2)
	assert.Equal(t, "token-2", token2)

	token3, found3 := cache.Get("uid-3")
	assert.True(t, found3)
	assert.Equal(t, "token-3", token3)

	// Verify non-existent UID returns not found
	_, found := cache.Get("uid-4")
	assert.False(t, found)
}

func TestCache_Overwrite(t *testing.T) {
	cache := NewCache()
	defer cache.Stop()

	uid := "test-uid"
	expiresAt := time.Now().Add(1 * time.Hour)

	// Set initial token
	cache.Set(uid, "token-1", expiresAt)
	token1, found := cache.Get(uid)
	assert.True(t, found)
	assert.Equal(t, "token-1", token1)

	// Overwrite with new token
	cache.Set(uid, "token-2", expiresAt)
	token2, found := cache.Get(uid)
	assert.True(t, found)
	assert.Equal(t, "token-2", token2, "token should be overwritten")
}

func TestCache_BackgroundCleanup(t *testing.T) {
	// This test verifies that the background cleanup goroutine actually runs
	// We'll use a short cleanup interval for testing purposes

	cache := &Cache{
		entries: make(map[string]cacheEntry),
		stopCh:  make(chan struct{}),
	}

	// Start with custom ticker for faster testing
	ticker := time.NewTicker(100 * time.Millisecond)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				cache.cleanup()
			case <-cache.stopCh:
				return
			}
		}
	}()
	defer cache.Stop()

	// Add expired entry
	cache.Set("expired-uid", "expired-token", time.Now().Add(-1*time.Hour))

	// Wait for background cleanup to run
	time.Sleep(200 * time.Millisecond)

	// Verify entry was removed by background cleanup
	cache.mu.RLock()
	_, found := cache.entries["expired-uid"]
	cache.mu.RUnlock()
	assert.False(t, found, "expired entry should be removed by background cleanup")
}

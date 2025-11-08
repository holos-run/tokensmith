package token

import (
	"sync"
	"time"
)

// cacheEntry stores a cached token with its expiration time.
type cacheEntry struct {
	token     string
	expiresAt time.Time
}

// Cache provides thread-safe caching of tokens indexed by workload service account UID.
// It automatically removes expired entries via background garbage collection.
type Cache struct {
	mu       sync.RWMutex
	entries  map[string]cacheEntry
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewCache creates a new cache and starts the background garbage collection goroutine.
// The garbage collector runs every 5 minutes to remove expired entries.
// Call Stop() when done to clean up the background goroutine.
func NewCache() *Cache {
	c := &Cache{
		entries: make(map[string]cacheEntry),
		stopCh:  make(chan struct{}),
	}
	c.start()
	return c
}

// Get retrieves a token from the cache by workload service account UID.
// Returns (token, true) if found and not expired, or ("", false) otherwise.
func (c *Cache) Get(uid string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, found := c.entries[uid]
	if !found {
		return "", false
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		return "", false
	}

	return entry.token, true
}

// Set stores a token in the cache indexed by workload service account UID.
// The token will be cached until expiresAt.
func (c *Cache) Set(uid string, token string, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[uid] = cacheEntry{
		token:     token,
		expiresAt: expiresAt,
	}
}

// Stop gracefully shuts down the background garbage collection goroutine.
// This should be called when the cache is no longer needed.
// It is safe to call Stop() multiple times.
func (c *Cache) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
}

// start begins the background garbage collection goroutine.
// Runs every 5 minutes to remove expired entries.
func (c *Cache) start() {
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.cleanup()
			case <-c.stopCh:
				return
			}
		}
	}()
}

// cleanup removes all expired entries from the cache.
// This is called periodically by the background goroutine.
func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for uid, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, uid)
		}
	}
}

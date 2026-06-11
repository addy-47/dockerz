package cache

import (
	"fmt"
	"sync"
	"time"

	"github.com/addy-47/dockerz/internal/logging"
)

// InMemoryCache implements CacheManager with in-memory storage.
// Useful for hot cache hits within a single build process.
type InMemoryCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
	logger  *logging.Logger
}

// NewInMemoryCache creates a new in-memory cache instance
func NewInMemoryCache() *InMemoryCache {
	return &InMemoryCache{
		entries: make(map[string]*CacheEntry),
		logger:  nil, // Will be set by caller
	}
}

// SetLogger sets the logger for the cache
func (im *InMemoryCache) SetLogger(logger *logging.Logger) {
	im.logger = logger
}

// Get retrieves a cache entry from in-memory cache
func (im *InMemoryCache) Get(serviceName string) (*CacheEntry, bool) {
	im.mu.RLock()
	entry, exists := im.entries[serviceName]
	im.mu.RUnlock()

	if !exists {
		if im.logger != nil {
			im.logger.Debug(logging.CATEGORY_CACHE, fmt.Sprintf("In-memory cache miss for %s", serviceName))
		}
		return nil, false
	}

	// Check if entry is expired
	if time.Since(entry.Timestamp) > entry.TTL {
		if im.logger != nil {
			im.logger.Debug(logging.CATEGORY_CACHE, fmt.Sprintf("In-memory cache expired for %s (age: %v)", serviceName, time.Since(entry.Timestamp)))
		}
		im.Clear(serviceName)
		return nil, false
	}

	if im.logger != nil {
		im.logger.Debug(logging.CATEGORY_CACHE, fmt.Sprintf("In-memory cache hit for %s", serviceName))
	}
	return entry, true
}

// Set stores a cache entry in in-memory cache
func (im *InMemoryCache) Set(entry *CacheEntry) error {
	im.mu.Lock()
	im.entries[entry.ServiceName] = entry
	im.mu.Unlock()

	if im.logger != nil {
		im.logger.Debug(logging.CATEGORY_CACHE, fmt.Sprintf("In-memory cache set for %s (hash: %s)", entry.ServiceName, entry.ImageHash))
	}
	return nil
}

// Clear removes a cache entry from in-memory cache
func (im *InMemoryCache) Clear(serviceName string) error {
	im.mu.Lock()
	delete(im.entries, serviceName)
	im.mu.Unlock()
	return nil
}

// Cleanup removes all expired cache entries
func (im *InMemoryCache) Cleanup() error {
	im.mu.Lock()
	defer im.mu.Unlock()

	for name, entry := range im.entries {
		if time.Since(entry.Timestamp) > entry.TTL {
			delete(im.entries, name)
		}
	}
	return nil
}

// Len returns the number of entries in the cache
func (im *InMemoryCache) Len() int {
	im.mu.RLock()
	defer im.mu.RUnlock()
	return len(im.entries)
}

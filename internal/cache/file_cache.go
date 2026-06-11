package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/addy-47/dockerz/internal/logging"
)

// FileCache implements CacheManager using file-based persistence
type FileCache struct {
	cacheDir string
	config   *CacheConfig
	logger   *logging.Logger
}

// NewFileCache creates a new file cache instance
func NewFileCache(config *CacheConfig) *FileCache {
	cacheDir := filepath.Join(os.TempDir(), "dockerz-file-cache")
	os.MkdirAll(cacheDir, 0755)

	return &FileCache{
		cacheDir: cacheDir,
		config:   config,
		logger:   nil, // Will be set by caller
	}
}

// SetLogger sets the logger for the cache
func (f *FileCache) SetLogger(logger *logging.Logger) {
	f.logger = logger
}

// Get retrieves a cache entry from file cache
func (f *FileCache) Get(serviceName string) (*CacheEntry, bool) {
	// Check file-based cache
	cacheFile := filepath.Join(f.cacheDir, fmt.Sprintf("%s.json", serviceName))

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		if f.logger != nil {
			f.logger.Debug(logging.CATEGORY_CACHE, fmt.Sprintf("Cache miss for %s: file not found", serviceName))
		}
		return nil, false
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		if f.logger != nil {
			f.logger.Debug(logging.CATEGORY_CACHE, fmt.Sprintf("Cache miss for %s: invalid cache data", serviceName))
		}
		return nil, false
	}

	// Check if entry is expired
	if time.Since(entry.Timestamp) > entry.TTL {
		if f.logger != nil {
			f.logger.Debug(logging.CATEGORY_CACHE, fmt.Sprintf("Cache miss for %s: expired (age: %v)", serviceName, time.Since(entry.Timestamp)))
		}
		f.Clear(serviceName) // Clean up expired entry
		return nil, false
	}

	if f.logger != nil {
		f.logger.Info(logging.CATEGORY_CACHE, fmt.Sprintf("File cache hit for %s (age: %v)", serviceName, time.Since(entry.Timestamp)))
	}
	return &entry, true
}

// Set stores a cache entry in file cache
func (f *FileCache) Set(entry *CacheEntry) error {
	cacheFile := filepath.Join(f.cacheDir, fmt.Sprintf("%s.json", entry.ServiceName))

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry for %s: %w", entry.ServiceName, err)
	}

	if f.logger != nil {
		f.logger.Debug(logging.CATEGORY_CACHE, fmt.Sprintf("Cache set for %s (hash: %s)", entry.ServiceName, entry.ImageHash))
	}

	return os.WriteFile(cacheFile, data, 0644)
}

// Clear removes a cache entry
func (f *FileCache) Clear(serviceName string) error {
	cacheFile := filepath.Join(f.cacheDir, fmt.Sprintf("%s.json", serviceName))
	return os.Remove(cacheFile)
}

// Cleanup removes all expired cache entries
func (f *FileCache) Cleanup() error {
	entries, err := os.ReadDir(f.cacheDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			serviceName := entry.Name()[:len(entry.Name())-5] // Remove .json extension
			if cached, exists := f.Get(serviceName); !exists || cached == nil {
				// Entry doesn't exist or is expired, remove file
				os.Remove(filepath.Join(f.cacheDir, entry.Name()))
			}
		}
	}

	return nil
}

// GetCacheStats returns statistics about the cache
func (f *FileCache) GetCacheStats() map[string]interface{} {
	return map[string]interface{}{
		"cache_dir": f.cacheDir,
	}
}

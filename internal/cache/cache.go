package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rvben/ru/internal/utils"
)

const cacheDirName = ".cache/ru"
const cacheExpiry = 60 * time.Minute

type Cache struct {
	data map[string]CacheItem
	mu   sync.RWMutex
}

type CacheItem struct {
	Version   string
	Timestamp time.Time
}

func NewCache() *Cache {
	return &Cache{
		data: make(map[string]CacheItem),
	}
}

func (c *Cache) getCacheDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, cacheDirName), nil
}

func (c *Cache) Load() error {
	cacheDir, err := c.getCacheDir()
	if err != nil {
		return err
	}
	cacheFile := filepath.Join(cacheDir, "cache.json")
	file, err := os.Open(cacheFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer file.Close()

	c.mu.Lock()
	defer c.mu.Unlock()
	return json.NewDecoder(file).Decode(&c.data)
}

func (c *Cache) Save() error {
	cacheDir, err := c.getCacheDir()
	if err != nil {
		return err
	}
	cacheFile := filepath.Join(cacheDir, "cache.json")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return err
	}
	file, err := os.Create(cacheFile)
	if err != nil {
		return err
	}
	defer file.Close()

	c.mu.RLock()
	defer c.mu.RUnlock()
	return json.NewEncoder(file).Encode(c.data)
}

func (c *Cache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.data[key]
	if !exists {
		return "", false
	}

	if time.Since(item.Timestamp) > cacheExpiry {
		return "", false
	}

	return item.Version, true
}

func (c *Cache) Set(key, version string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = CacheItem{
		Version:   version,
		Timestamp: time.Now(),
	}
}

// getCacheDir returns the path to the cache directory
func getCacheDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, cacheDirName), nil
}

func Clean() error {
	// Get the cache directory
	cacheDir, err := getCacheDir()
	if err != nil {
		return fmt.Errorf("failed to get cache directory: %w", err)
	}

	// Create the cache directory if it doesn't exist
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		return nil // No cache to clean
	}

	// Remove all files in the cache directory
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return fmt.Errorf("failed to read cache directory: %w", err)
	}

	for _, entry := range entries {
		filePath := filepath.Join(cacheDir, entry.Name())
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("failed to remove cache file %s: %w", filePath, err)
		}
	}

	// Also clear the version cache
	utils.ClearVersionCache()

	return nil
}

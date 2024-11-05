package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
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

func Clean() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	cacheDir := filepath.Join(homeDir, cacheDirName)
	if err := os.RemoveAll(cacheDir); err != nil {
		return fmt.Errorf("failed to remove cache directory: %w", err)
	}
	return nil
}

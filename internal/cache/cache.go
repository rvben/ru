package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const cacheDirName = ".cache/ru"
const cacheExpiry = 60 * time.Minute

type Cache struct {
	data map[string]CacheItem
}

type CacheItem struct {
	Version   string
	Timestamp time.Time
}

func NewCache() *Cache {
	return &Cache{data: make(map[string]CacheItem)}
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

	return json.NewEncoder(file).Encode(c.data)
}

func (c *Cache) Get(packageName string) (string, bool) {
	item, found := c.data[packageName]
	if !found || time.Since(item.Timestamp) > cacheExpiry {
		return "", false
	}
	return item.Version, true
}

func (c *Cache) Set(packageName, version string) {
	c.data[packageName] = CacheItem{
		Version:   version,
		Timestamp: time.Now(),
	}
}

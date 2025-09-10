package cache

import (
	"encoding/json"
	"sync"
	"time"
)

type CacheEntry struct {
	Data      interface{}
	Timestamp time.Time
	TTL       time.Duration
}

type Cache interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{}, ttl time.Duration)
	Delete(key string)
	Clear()
	IsExpired(key string) bool
}

type MemoryCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
}

func NewMemoryCache() *MemoryCache {
	cache := &MemoryCache{
		entries: make(map[string]*CacheEntry),
	}
	go cache.cleanupRoutine()
	return cache
}

func (c *MemoryCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil, false
	}

	if time.Since(entry.Timestamp) > entry.TTL {
		return nil, false
	}

	return entry.Data, true
}

func (c *MemoryCache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &CacheEntry{
		Data:      value,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
}

func (c *MemoryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

func (c *MemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*CacheEntry)
}

func (c *MemoryCache) IsExpired(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return true
	}

	return time.Since(entry.Timestamp) > entry.TTL
}

func (c *MemoryCache) cleanupRoutine() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		for key, entry := range c.entries {
			if time.Since(entry.Timestamp) > entry.TTL {
				delete(c.entries, key)
			}
		}
		c.mu.Unlock()
	}
}

type CacheKeyBuilder struct {
	parts []string
}

func NewCacheKeyBuilder() *CacheKeyBuilder {
	return &CacheKeyBuilder{
		parts: make([]string, 0),
	}
}

func (b *CacheKeyBuilder) Add(part string) *CacheKeyBuilder {
	b.parts = append(b.parts, part)
	return b
}

func (b *CacheKeyBuilder) Build() string {
	result := ""
	for i, part := range b.parts {
		if i > 0 {
			result += ":"
		}
		result += part
	}
	return result
}

func CacheJSON(cache Cache, key string, ttl time.Duration, fn func() (interface{}, error)) (interface{}, error) {
	if data, found := cache.Get(key); found {
		return data, nil
	}

	result, err := fn()
	if err != nil {
		return nil, err
	}

	cache.Set(key, result, ttl)
	return result, nil
}

func SerializeForCache(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func DeserializeFromCache(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
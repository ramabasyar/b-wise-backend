package sso

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrCacheMiss is returned when a cache key is not found
var ErrCacheMiss = fmt.Errorf("cache miss")

// Cache handles caching of SSO responses in Redis
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string) error
}

// RedisCache implements Cache using Redis
type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedisCache creates a new Redis-backed SSO cache
func NewRedisCache(client *redis.Client, ttl time.Duration) *RedisCache {
	return &RedisCache{
		client: client,
		ttl:    ttl,
	}
}

// Get retrieves a cached value
func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, "cache:"+key).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("cache get error: %w", err)
	}
	return val, nil
}

// Set stores a value in cache
func (c *RedisCache) Set(ctx context.Context, key string, value string) error {
	return c.client.Set(ctx, "cache:"+key, value, c.ttl).Err()
}

// GetJSON retrieves and unmarshals a cached JSON value
func GetJSON(ctx context.Context, cache Cache, key string, dest interface{}) error {
	val, err := cache.Get(ctx, key)
	if err != nil || val == "" {
		return fmt.Errorf("cache miss")
	}
	return json.Unmarshal([]byte(val), dest)
}

// SetJSON marshals and stores a value in cache
func SetJSON(ctx context.Context, cache Cache, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache marshal error: %w", err)
	}
	return cache.Set(ctx, key, string(data))
}

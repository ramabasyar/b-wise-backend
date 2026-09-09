package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config holds cache configuration
type Config struct {
	Host         string
	Port         int
	Password     string
	DB           int
	PoolSize     int
	MinIdleConns int
}

// Cache wraps Redis client
type Cache struct {
	client *redis.Client
}

// New creates a new cache
func New(cfg Config) (*Cache, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &Cache{client: rdb}, nil
}

// Client returns the underlying Redis client
func (c *Cache) Client() *redis.Client {
	return c.client
}

// Close closes the cache connection
func (c *Cache) Close() error {
	return c.client.Close()
}

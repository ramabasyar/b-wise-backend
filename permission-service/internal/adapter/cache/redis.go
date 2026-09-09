package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	keyPermPrefix   = "perm:"   // perm:{userID}:{serviceID}:{permission} -> "1"
	keyMenuPrefix   = "menu:"   // menu:{userID} -> json
	keyAccessPrefix = "access:" // access:{userID}:{serviceID} -> "1"
	keyUserPrefix   = "userp:"  // userp:{userID} -> set of cache keys (untuk invalidasi)

	permTTL = 2 * time.Minute
	menuTTL = 2 * time.Minute
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

// ==================== Permission Cache Layer ====================



// cacheSetString helper
func (c *Cache) cacheSetString(ctx context.Context, key, val string, ttl time.Duration) error {
	return c.client.Set(ctx, key, val, ttl).Err()
}

// GetCachedPermission returns (has, found) — found=false berarti cache miss
func (c *Cache) GetCachedPermission(ctx context.Context, userID, serviceID, permission string) (bool, bool) {
	val, err := c.client.Get(ctx, keyPermPrefix+userID+":"+serviceID+":"+permission).Result()
	if err != nil {
		return false, false // miss (atau redis error → treat miss, fail-open ke DB)
	}
	return val == "1", true
}

// SetCachedPermission simpan hasil permission check (TTL singkat)
func (c *Cache) SetCachedPermission(ctx context.Context, userID, serviceID, permission string, has bool) {
	val := "0"
	if has {
		val = "1"
	}
	_ = c.cacheSetString(ctx, keyPermPrefix+userID+":"+serviceID+":"+permission, val, permTTL)
	c.trackUserKey(ctx, userID, keyPermPrefix+userID+":"+serviceID+":"+permission)
}

// GetCachedMenu returns menu JSON; found=false = miss
func (c *Cache) GetCachedMenu(ctx context.Context, userID string) ([]byte, bool) {
	val, err := c.client.Get(ctx, keyMenuPrefix+userID).Bytes()
	if err != nil {
		return nil, false
	}
	return val, true
}

// SetCachedMenu simpan menu user
func (c *Cache) SetCachedMenu(ctx context.Context, userID string, menuJSON []byte) {
	_ = c.cacheSetString(ctx, keyMenuPrefix+userID, string(menuJSON), menuTTL)
	c.trackUserKey(ctx, userID, keyMenuPrefix+userID)
}

// trackUserKey catat key milik user (SET) untuk invalidasi massal
func (c *Cache) trackUserKey(ctx context.Context, userID, key string) {
	c.client.SAdd(ctx, keyUserPrefix+userID, key)
	c.client.Expire(ctx, keyUserPrefix+userID, permTTL)
}

// InvalidateUser hapus SEMUA cache milik user (saat role/permission/access berubah)
func (c *Cache) InvalidateUser(ctx context.Context, userID string) error {
	keys, err := c.client.SMembers(ctx, keyUserPrefix+userID).Result()
	if err == nil && len(keys) > 0 {
		if delErr := c.client.Del(ctx, keys...).Err(); delErr != nil {
			return delErr
		}
	}
	return c.client.Del(ctx, keyUserPrefix+userID).Err()
}


// GetCachedAccess returns (has, found) — found=false berarti cache miss
func (c *Cache) GetCachedAccess(ctx context.Context, userID, serviceID string) (bool, bool) {
	val, err := c.client.Get(ctx, keyAccessPrefix+userID+":"+serviceID).Result()
	if err != nil {
		return false, false
	}
	return val == "1", true
}

// SetCachedAccess simpan hasil access check
func (c *Cache) SetCachedAccess(ctx context.Context, userID, serviceID string, has bool) {
	val := "0"
	if has {
		val = "1"
	}
	_ = c.cacheSetString(ctx, keyAccessPrefix+userID+":"+serviceID, val, permTTL)
	c.trackUserKey(ctx, userID, keyAccessPrefix+userID+":"+serviceID)
}

// FlushPattern hapus SEMUA key dengan prefix (mis. "menu:") — dipakai saat
// struktur menu berubah (self-sync) karena semua user terdampak.
func (c *Cache) FlushPattern(ctx context.Context, prefix string) error {
	iter := c.client.Scan(ctx, 0, prefix+"*", 100).Iterator()
	var keys []string
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return err
	}
	if len(keys) > 0 {
		return c.client.Del(ctx, keys...).Err()
	}
	return nil
}

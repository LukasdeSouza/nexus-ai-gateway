// Package redisstore wraps the go-redis client with helpers for rate limiting and key caching.
package redisstore

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/LukasdeSouza/nexus-ai-gateway/internal/config"
)

// Client wraps redis.Client with helper methods.
type Client struct {
	rdb *redis.Client
}

// NewClient initializes a Redis client connection and validates via Ping.
func NewClient(cfg config.RedisConfig) (*Client, error) {
	opts, err := redis.ParseURL(cfg.URL)
	if err != nil {
		opts = &redis.Options{
			Addr:     cfg.URL,
			Password: cfg.Password,
			DB:       cfg.DB,
		}
	}
	if cfg.Password != "" {
		opts.Password = cfg.Password
	}
	if cfg.DB != 0 {
		opts.DB = cfg.DB
	}

	rdb := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &Client{rdb: rdb}, nil
}

// NewClientFromRedis wraps an existing redis.Client instance.
func NewClientFromRedis(rdb *redis.Client) *Client {
	return &Client{rdb: rdb}
}

// SetWithTTL sets a key with a time-to-live.
func (c *Client) SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

// Get gets a key value, returning empty string without error if key does not exist.
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	val, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// Delete removes one or more keys.
func (c *Client) Delete(ctx context.Context, keys ...string) error {
	return c.rdb.Del(ctx, keys...).Err()
}

// IncrementWithExpiry increments a key and applies a TTL if the key is newly created.
func (c *Client) IncrementWithExpiry(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	pipe := c.rdb.Pipeline()
	incrCmd := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return incrCmd.Val(), nil
}

// Close closes the underlying Redis connection.
func (c *Client) Close() error {
	return c.rdb.Close()
}

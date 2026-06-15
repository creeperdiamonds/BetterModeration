package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Cache wraps a go-redis client for cache operations.
type Cache struct {
	client *redis.Client
}

// New creates a new Cache by parsing the provided Redis URL and creating a client.
func New(url string) (*Cache, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parsing redis URL: %w", err)
	}

	client := redis.NewClient(opts)
	return &Cache{client: client}, nil
}

// Close closes the underlying Redis connection.
func (c *Cache) Close() error {
	if err := c.client.Close(); err != nil {
		return fmt.Errorf("closing redis client: %w", err)
	}
	return nil
}

// Ping verifies the connection to Redis is alive.
func (c *Cache) Ping(ctx context.Context) error {
	if err := c.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}

// Client returns the underlying redis.Client for direct use (e.g. EventBus).
func (c *Cache) Client() *redis.Client {
	return c.client
}

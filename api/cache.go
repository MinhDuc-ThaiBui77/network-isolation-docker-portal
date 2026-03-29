package main

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

var rdb *redis.Client

var ctx = context.Background()

// ConnectRedis connects to Redis using a URL like "redis://host:6379".
func ConnectRedis(url string) error {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return err
	}
	rdb = redis.NewClient(opt)
	return rdb.Ping(ctx).Err()
}

// CacheGet returns the cached value and true if found, or ("", false) on miss.
func CacheGet(key string) (string, bool) {
	if rdb == nil {
		return "", false
	}
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return "", false
	}
	return val, true
}

// CacheSet stores a value with a TTL.
func CacheSet(key, value string, ttl time.Duration) {
	if rdb == nil {
		return
	}
	if err := rdb.Set(ctx, key, value, ttl).Err(); err != nil {
		log.Printf("Failed to cache set: %v", err)
	}
}

// CacheInvalidate deletes a cached key.
func CacheInvalidate(key string) {
	if rdb == nil {
		return
	}
	rdb.Del(ctx, key)
}

// CloseRedis closes the Redis connection.
func CloseRedis() {
	if rdb != nil {
		rdb.Close()
	}
}

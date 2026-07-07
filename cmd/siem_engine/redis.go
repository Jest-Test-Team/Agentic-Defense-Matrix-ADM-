package main

import (
	"context"
	"os"
	"time"

	"github.com/go-redis/redis/v9"
)

type RedisClient struct {
	Client *redis.Client
}

func NewRedisClient() (*RedisClient, error) {
	redisURL := os.Getenv("ADM_REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &RedisClient{Client: client}, nil
}

func (r *RedisClient) IsHealthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return r.Client.Ping(ctx).Err() == nil
}

func (r *RedisClient) PublishEvent(ctx context.Context, event interface{}) error {
	return r.Client.RPush(ctx, "siem:events", event).Err()
}

func (r *RedisClient) GetRecentEvents(ctx context.Context, count int64) ([]string, error) {
	return r.Client.LRange(ctx, "siem:events", 0, count-1).Result()
}

package cache

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var RedisClient *redis.Client

var ErrRedisNotAvailable = errors.New("Redis 未连接")

func Init() error {
	RedisClient = redis.NewClient(&redis.Options{
		Addr:     os.Getenv("REDIS_URL"),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := RedisClient.Ping(ctx).Result()
	if err != nil {
		RedisClient = nil
		return err
	}
	return nil
}

func Set(key string, value interface{}, expiration time.Duration) error {
	if RedisClient == nil {
		return ErrRedisNotAvailable
	}
	ctx := context.Background()
	return RedisClient.Set(ctx, key, value, expiration).Err()
}

func Get(key string) (string, error) {
	if RedisClient == nil {
		return "", ErrRedisNotAvailable
	}
	ctx := context.Background()
	return RedisClient.Get(ctx, key).Result()
}

func Delete(key string) error {
	if RedisClient == nil {
		return ErrRedisNotAvailable
	}
	ctx := context.Background()
	return RedisClient.Del(ctx, key).Err()
}

func Exists(key string) (bool, error) {
	if RedisClient == nil {
		return false, ErrRedisNotAvailable
	}
	ctx := context.Background()
	result, err := RedisClient.Exists(ctx, key).Result()
	return result > 0, err
}

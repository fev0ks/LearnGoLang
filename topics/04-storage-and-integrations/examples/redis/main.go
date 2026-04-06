package main

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"time"
)

func main() {
	ctx := context.Background()
	currentTime := time.Now().UnixNano()
	redisClient := newRedisClient(getConfig())

	key := "redis_key"

	pipe := redisClient.TxPipeline()
	pipe.ZRemRangeByScore(ctx, key)

	pipe.ZAdd(ctx, key, &redis.Z{
		Score:  float64(currentTime),
		Member: currentTime,
	})
}

func newRedisClient(config RedisConfig) *redis.Client {
	address := fmt.Sprintf("%s:%s", config.Host, config.Port)
	return redis.NewClient(&redis.Options{
		Addr:     address,
		Password: config.Password,
		DB:       config.DB,
		Username: config.User,
	})
}

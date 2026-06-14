package external

import (
	"context"

	"github.com/redis/go-redis/v9"
)

var RedisClient redis.UniversalClient

func init() {
	RedisClient = redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs: []string{"127.0.0.1:6379"},
		DB:    0,
	})
	_, err := RedisClient.Ping(context.Background()).Result()
	if err != nil {
		panic(err)
	}
}

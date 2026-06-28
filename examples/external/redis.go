package external

import "github.com/redis/go-redis/v9"

var rc redis.UniversalClient

func init() {
	rc = redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs: []string{"127.0.0.1:6379"},
	})
}
func RedisClient() redis.UniversalClient {
	return rc
}

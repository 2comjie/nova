package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type Discovery struct {
	rc     redis.UniversalClient
	option *option

	ctx    context.Context
	cancel context.CancelFunc
}

func (r *Discovery) keepAlive(aliveKey string) {

}

package main

import (
	"context"
	"errors"

	"github.com/2comjie/wali/core/util"
	"github.com/2comjie/wali/examples/internal/external"
	redisLocator "github.com/2comjie/wali/locator/redis"
	"github.com/2comjie/wali/logx"
	"github.com/redis/go-redis/v9"
)

func main() {
	locator := redisLocator.NewProvider(external.RedisClient)

	// bind 和 locate
	err := locator.Bind(context.Background(), "user", "user-1", "user-service-1")
	if err != nil {
		panic(err)
	}
	err = locator.Bind(context.Background(), "user", "user-2", "user-service-2")
	if err != nil {
		panic(err)
	}
	err = locator.Bind(context.Background(), "game", "user-1", "game-service-1")
	if err != nil {
		panic(err)
	}
	err = locator.Bind(context.Background(), "game", "user-2", "game-service-2")
	if err != nil {
		panic(err)
	}

	ins, err := locator.Locate(context.Background(), "user", "user-1")
	if err != nil {
		panic(err)
	}
	logx.Infof("ins %v", ins)
	ins, err = locator.Locate(context.Background(), "user", "user-2")
	if err != nil {
		panic(err)
	}
	logx.Infof("ins %v", ins)
	ins, err = locator.Locate(context.Background(), "game", "user-1")
	if err != nil {
		panic(err)
	}
	logx.Infof("ins %v", ins)
	ins, err = locator.Locate(context.Background(), "game", "user-2")
	if err != nil {
		panic(err)
	}
	logx.Infof("ins %v", ins)
	ins, err = locator.Locate(context.Background(), "game", "user-3")
	if err != nil && !errors.Is(err, redis.Nil) {
		panic(err)
	}
	logx.Infof("ins %v", ins)
	util.WaitUntilSignaled()
	locator.Close()
}

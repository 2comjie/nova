package main

import (
	"context"

	"github.com/2comjie/wali/core/util"
	"github.com/2comjie/wali/examples/external"
	"github.com/2comjie/wali/locator"
	redisLocator "github.com/2comjie/wali/locator/redis"
	"github.com/2comjie/wali/logx"
)

func main() {
	rdb := external.RedisClient()
	loc := locator.NewNodeLocator(redisLocator.NewProvider(rdb))
	err := loc.Bind(context.Background(), "demo", "uid-01", "demo-1")
	if err != nil {
		panic(err)
	}
	err = loc.Bind(context.Background(), "demo", "uid-02", "demo-2")
	if err != nil {
		panic(err)
	}

	id, err := loc.Locate(context.Background(), "demo", "uid-01")
	if err != nil {
		panic(err)
	} else {
		logx.Infof("locate: %s", id)
	}
	id, err = loc.Locate(context.Background(), "demo", "uid-02")
	if err != nil {
		panic(err)
	} else {
		logx.Infof("locate: %s", id)
	}
	util.WaitUntilSignaled()
}

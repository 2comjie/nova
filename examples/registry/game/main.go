package main

import (
	"context"

	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/core/util"
	"github.com/2comjie/wali/examples/registry/internal/external"
	"github.com/2comjie/wali/logx"
	redisRegistry "github.com/2comjie/wali/registry/redis"
)

func main() {
	register := redisRegistry.NewRegistry(external.RedisClient)
	ins := endpoint.ServiceInstance{
		ID:          "game",
		MetaData:    nil,
		Weight:      10,
		Status:      0,
		RpcServices: nil,
		Routes:      nil,
	}
	err := register.Register(ins)
	if err != nil {
		panic(err)
	}
	dis := redisRegistry.NewDiscover(external.RedisClient)

	ctx, cancle := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				instances, err := dis.Next(ctx)
				if err != nil {
					panic(err)
				}
				logx.Infof("instances %v", instances)
			}
		}
	}()

	util.WaitUntilSignaled()
	cancle()
}

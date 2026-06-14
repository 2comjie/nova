package main

import (
	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/core/util"
	"github.com/2comjie/wali/examples/internal/external"
	redisRegistry "github.com/2comjie/wali/registry/redis"
)

func main() {
	register := redisRegistry.NewRegistry(external.RedisClient)
	ins := endpoint.ServiceInstance{
		ID:          "api",
		MetaData:    nil,
		Weight:      0,
		Status:      0,
		RpcServices: nil,
		Routes:      nil,
	}
	err := register.Register(ins)
	if err != nil {
		panic(err)
	}
	util.WaitUntilSignaled()
	register.Deregister(ins.ID)
	register.Close()
}

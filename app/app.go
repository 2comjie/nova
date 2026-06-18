package app

import (
	"github.com/2comjie/wali/core/endpoint"
	"google.golang.org/grpc"
)

type App interface {
	grpc.ServiceRegistrar
}

type baseApp struct {
	rpcServer       *grpc.Server // 继承了 grpcServer
	serviceInstance endpoint.ServiceInstance
}

func (a *baseApp) RegisterService(desc *grpc.ServiceDesc, impl any) {
	a.rpcServer.RegisterService(desc, impl)
	a.serviceInstance.RpcServices[desc.ServiceName] = endpoint.RpcService{
		Name: desc.ServiceName,
		Desc: desc.ServiceName,
	}
}

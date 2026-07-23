package app

import (
	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/registry"
	"google.golang.org/grpc"
)

type App interface {
	Start() error
	Stop() error
}

type baseApp struct {
	rpcServer *grpc.Server

	registry registry.Registry
	discover registry.Discover

	gateLocator locator.GateLocator
	nodeLocator locator.NodeLocator
}

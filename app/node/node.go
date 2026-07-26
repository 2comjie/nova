package node

import (
	"github.com/2comjie/wali/core/endpoint"
	pbGate "github.com/2comjie/wali/internal/pb/transport/gate"
	"github.com/2comjie/wali/locator"
)

// Node 管理一个业务Node实例。
// RPC服务、Router和生命周期将在Node App实现时继续补充。
type Node struct {
	instance    endpoint.ServiceInstance
	nodeLocator *locator.NodeLocator
	gateLocator *locator.GateLocator
	gateClient  pbGate.GateClient
	proxy       *Proxy
}

package deploy

import (
	"context"
	"errors"
	"maps"

	nodeapp "github.com/2comjie/wali/app/node"
	"github.com/2comjie/wali/config"
	"github.com/2comjie/wali/core/endpoint"
	pbGate "github.com/2comjie/wali/internal/pb/transport/gate"
	"github.com/2comjie/wali/locator"
)

// NodeApp 是deploy装配完成的Node服务。
type NodeApp struct {
	*nodeapp.Node

	instance  endpoint.ServiceInstance
	resources *resources
}

// Node 使用启动参数和Option装配Node服务。
func Node(opts ...Option) (*NodeApp, error) {
	options := defaultOptions()
	for _, option := range opts {
		option(&options)
	}
	if options.nodeRouter == nil {
		options.nodeRouter = nodeapp.NewRouter()
	}

	resources, instance, rpcServer, err := buildResources(options)
	if err != nil {
		return nil, err
	}
	app, err := nodeapp.New(nodeapp.Config{
		Instance:    instance,
		Router:      options.nodeRouter,
		NodeLocator: locator.NewNodeLocator(options.locator),
		GateLocator: locator.NewGateLocator(options.locator, options.discover),
		GateClient:  pbGate.NewGateClient(resources.rpcClient),
		Registry:    options.registry,
		RPCServer:   rpcServer,
		RPCListener: resources.rpcListener,
		Components:  options.components,
	})
	if err != nil {
		return nil, errors.Join(err, resources.close())
	}
	return &NodeApp{
		Node:      app,
		instance:  instance,
		resources: resources,
	}, nil
}

// Run 启动Node，收到SIGINT或SIGTERM后执行优雅停机。
func (n *NodeApp) Run() error {
	return n.resources.run(n.Start, n.Shutdown)
}

// Shutdown 关闭Node和deploy创建的配置、RPC及发现资源。
func (n *NodeApp) Shutdown(ctx context.Context) error {
	return errors.Join(n.Node.Shutdown(ctx), n.resources.close())
}

// Config 返回已经完成Load的配置中心。
func (n *NodeApp) Config() config.Config {
	return n.resources.config
}

// Instance 返回当前Node注册信息的副本。
func (n *NodeApp) Instance() endpoint.ServiceInstance {
	instance := n.instance
	instance.MetaData = maps.Clone(instance.MetaData)
	return instance
}

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

type NodeApp struct {
	*nodeapp.Node

	instance  endpoint.ServiceInstance
	resources *resources
}

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
	app := nodeapp.New(nodeapp.Config{
		Instance:    instance,
		Router:      options.nodeRouter,
		NodeLocator: locator.NewNodeLocator(options.locator),
		GateLocator: locator.NewGateLocator(options.locator, options.discover),
		GateClient:  pbGate.NewGateClient(resources.rpcClient),
		Registry:    options.registry,
		RPCServer:   rpcServer,
		RPCListener: resources.rpcListener,
		Components:  options.components,
		Discovery:   options.discover,
	})
	return &NodeApp{
		Node:      app,
		instance:  instance,
		resources: resources,
	}, nil
}

func (n *NodeApp) Run() error {
	return n.resources.run(n.Start, n.Shutdown)
}

func (n *NodeApp) Shutdown(ctx context.Context) error {
	return errors.Join(n.Node.Shutdown(ctx), n.resources.close())
}

func (n *NodeApp) Config() config.Config {
	return n.resources.config
}

func (n *NodeApp) Instance() endpoint.ServiceInstance {
	instance := n.instance
	instance.MetaData = maps.Clone(instance.MetaData)
	return instance
}

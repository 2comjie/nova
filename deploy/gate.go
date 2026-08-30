package deploy

import (
	"context"
	"errors"
	"maps"

	gateapp "github.com/2comjie/nova/app/gate"
	"github.com/2comjie/nova/config"
	"github.com/2comjie/nova/core/endpoint"
	pbGate "github.com/2comjie/nova/internal/pb/transport/gate"
	pbNode "github.com/2comjie/nova/internal/pb/transport/node"
	"github.com/2comjie/nova/locator"
)

type GateApp struct {
	*gateapp.Gate

	instance  endpoint.ServiceInstance
	resources *resources
}

func Gate(opts ...Option) (*GateApp, error) {
	options := defaultOptions()
	for _, option := range opts {
		option(&options)
	}
	if options.serviceName != "" && options.serviceName != locator.GateName {
		panic("deploy: Gate service必须为gate")
	}
	if options.gateRouter == nil {
		options.gateRouter = gateapp.NewRouter()
	}

	resources, instance, rpcServer, err := buildResources(options)
	if err != nil {
		return nil, err
	}
	if err := loadGateRoutes(resources.config, options.gateRouter); err != nil {
		_ = resources.close()
		return nil, err
	}

	app := gateapp.New(gateapp.Config{
		Instance:       instance,
		Router:         options.gateRouter,
		NodeClient:     pbNode.NewNodeClient(resources.rpcClient),
		GateClient:     pbGate.NewGateClient(resources.rpcClient),
		Locator:        locator.NewGateLocator(options.locator, options.discover),
		Registry:       options.registry,
		RPCServer:      rpcServer,
		RPCListener:    resources.rpcListener,
		NetworkOptions: options.networkOptions,
		Hooks:          options.hooks,
		ErrorHandler:   options.errorHandler,
		LocatorTimeout: options.locatorTimeout,
		Components:     options.components,
	})
	return &GateApp{
		Gate:      app,
		instance:  instance,
		resources: resources,
	}, nil
}

func (g *GateApp) Run() error {
	return g.resources.run(g.Start, g.Shutdown)
}

func (g *GateApp) Shutdown(ctx context.Context) error {
	err := g.Gate.Shutdown(ctx)
	closeErr := g.resources.close()
	if err != nil {
		return err
	}
	return closeErr
}

func (g *GateApp) Config() config.Config {
	return g.resources.config
}

func (g *GateApp) Instance() endpoint.ServiceInstance {
	instance := g.instance
	instance.MetaData = maps.Clone(instance.MetaData)
	return instance
}

func loadGateRoutes(configCenter config.Config, router *gateapp.Router) error {
	filters, err := config.Get[[]gateapp.FilterConfig](configCenter, "gate.filters")
	if err == nil {
		if err := router.Use(filters...); err != nil {
			return err
		}
	} else if !errors.Is(err, config.ErrNotFound) {
		return err
	}

	routes, err := config.Get[[]gateapp.Route](configCenter, "gate.routes")
	if err == nil {
		return router.Add(routes...)
	}
	if errors.Is(err, config.ErrNotFound) {
		return nil
	}
	return err
}

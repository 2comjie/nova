package deploy

import (
	"context"
	"errors"
	"maps"
	"strings"

	gateapp "github.com/2comjie/wali/app/gate"
	"github.com/2comjie/wali/config"
	"github.com/2comjie/wali/core/endpoint"
	pbNode "github.com/2comjie/wali/internal/pb/transport/node"
	"github.com/2comjie/wali/locator"
)

// GateApp 是deploy装配完成的Gate服务。
type GateApp struct {
	*gateapp.Gate

	instance  endpoint.ServiceInstance
	resources *resources
}

// Gate 使用启动参数和Option装配Gate服务。
func Gate(opts ...Option) (*GateApp, error) {
	options := defaultOptions()
	for _, option := range opts {
		option(&options)
	}
	serviceName := strings.TrimSpace(options.serviceName)
	if serviceName != "" && serviceName != locator.GateName {
		return nil, gateapp.ErrInstanceRequired
	}
	if options.gateRouter == nil {
		options.gateRouter = gateapp.NewRouter()
	}

	resources, instance, rpcServer, err := buildResources(options)
	if err != nil {
		return nil, err
	}
	if err := loadGateRoutes(resources.config, options.gateRouter); err != nil {
		return nil, errors.Join(err, resources.close())
	}

	app, err := gateapp.New(gateapp.Config{
		Instance:       instance,
		Router:         options.gateRouter,
		NodeClient:     pbNode.NewNodeClient(resources.rpcClient),
		Locator:        locator.NewGateLocator(options.locator, options.discover),
		Registry:       options.registry,
		RPCServer:      rpcServer,
		RPCListener:    resources.rpcListener,
		NetworkOptions: options.networkOptions,
		Hooks:          options.hooks,
		ErrorHandler:   options.errorHandler,
		LocatorTimeout: options.locatorTimeout,
	})
	if err != nil {
		return nil, errors.Join(err, resources.close())
	}
	return &GateApp{
		Gate:      app,
		instance:  instance,
		resources: resources,
	}, nil
}

// Run 启动Gate，收到SIGINT或SIGTERM后执行优雅停机。
func (g *GateApp) Run() error {
	return g.resources.run(g.Start, g.Shutdown)
}

// Shutdown 关闭Gate和deploy创建的配置、RPC及发现资源。
func (g *GateApp) Shutdown(ctx context.Context) error {
	return errors.Join(g.Gate.Shutdown(ctx), g.resources.close())
}

// Config 返回已经完成Load的配置中心。
func (g *GateApp) Config() config.Config {
	return g.resources.config
}

// Instance 返回当前Gate注册信息的副本。
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

package deploy

import (
	"context"
	"errors"
	"maps"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/2comjie/nova/config"
	"github.com/2comjie/nova/config/file"
	"github.com/2comjie/nova/core/endpoint"
	"github.com/2comjie/nova/core/util"
	novaflag "github.com/2comjie/nova/flag"
	"github.com/2comjie/nova/locator"
	"github.com/2comjie/nova/registry"
	rpcclient "github.com/2comjie/nova/rpc/client"
	"github.com/spf13/cast"
	"google.golang.org/grpc"
)

const defaultShutdownTimeout = 30 * time.Second

var (
	ErrServiceNameRequired = errors.New("deploy: 启动参数service不能为空")
	ErrInstanceIDRequired  = errors.New("deploy: 启动参数id不能为空")
	ErrConfigRequired      = errors.New("deploy: 必须提供Config")
	ErrRegistryRequired    = errors.New("deploy: 必须提供Registry")
	ErrDiscoverRequired    = errors.New("deploy: 必须提供Discover")
	ErrLocatorRequired     = errors.New("deploy: 必须提供Locator")
	ErrRPCAddress          = errors.New("deploy: RPC监听地址无效")
)

type resources struct {
	config          config.Config
	registry        registry.Registry
	discover        registry.Discover
	locator         locator.Locator
	rpcClient       *rpcclient.Client
	rpcListener     net.Listener
	shutdownTimeout time.Duration

	closeOnce sync.Once
	closeErr  error
}

func (r *resources) close() error {
	r.closeOnce.Do(func() {
		r.rpcClient.Close()
		r.discover.Close()
		r.locator.Close()
		r.registry.Close()
		_ = r.rpcListener.Close()
		r.closeErr = r.config.Close()
	})
	return r.closeErr
}

func (r *resources) run(start func() error, shutdown func(context.Context) error) error {
	if err := start(); err != nil {
		return errors.Join(err, r.close())
	}

	util.WaitUntilSignaled()

	ctx, cancel := context.WithTimeout(context.Background(), r.shutdownTimeout)
	defer cancel()
	return shutdown(ctx)
}

func buildResources(options options) (*resources, endpoint.ServiceInstance, *grpc.Server, error) {
	if strings.TrimSpace(options.serviceName) == "" {
		return nil, endpoint.ServiceInstance{}, nil, ErrServiceNameRequired
	}
	if strings.TrimSpace(options.instanceID) == "" {
		return nil, endpoint.ServiceInstance{}, nil, ErrInstanceIDRequired
	}
	if options.config == nil {
		return nil, endpoint.ServiceInstance{}, nil, ErrConfigRequired
	}
	if options.registry == nil {
		return nil, endpoint.ServiceInstance{}, nil, ErrRegistryRequired
	}
	if options.discover == nil {
		return nil, endpoint.ServiceInstance{}, nil, ErrDiscoverRequired
	}
	if options.locator == nil {
		return nil, endpoint.ServiceInstance{}, nil, ErrLocatorRequired
	}
	if err := options.config.Load(); err != nil {
		_ = options.config.Close()
		return nil, endpoint.ServiceInstance{}, nil, err
	}

	rpcListener := options.rpcListener
	if rpcListener == nil {
		var err error
		rpcListener, err = net.Listen("tcp", options.rpcListen)
		if err != nil {
			_ = options.config.Close()
			return nil, endpoint.ServiceInstance{}, nil, err
		}
	}

	rpcHost, rpcPort, err := rpcEndpoint(rpcListener, options.rpcHost)
	if err != nil {
		_ = rpcListener.Close()
		_ = options.config.Close()
		return nil, endpoint.ServiceInstance{}, nil, err
	}

	rpcServer := options.rpcServer
	if rpcServer == nil {
		rpcServer = grpc.NewServer(options.rpcServerOptions...)
	}
	client := options.rpcClient
	if client == nil {
		client = rpcclient.NewClient(
			options.discover,
			options.locator,
			options.rpcClientOptions...,
		)
	}
	resource := &resources{
		config:          options.config,
		registry:        options.registry,
		discover:        options.discover,
		locator:         options.locator,
		rpcClient:       client,
		rpcListener:     rpcListener,
		shutdownTimeout: options.shutdownTimeout,
	}
	instance := endpoint.ServiceInstance{
		ID:          strings.TrimSpace(options.instanceID),
		ServiceName: strings.TrimSpace(options.serviceName),
		MetaData:    maps.Clone(options.metaData),
		Weight:      options.weight,
		RpcHost:     rpcHost,
		RpcPort:     rpcPort,
		Status:      endpoint.Working,
	}
	return resource, instance, rpcServer, nil
}

func rpcEndpoint(listener net.Listener, configuredHost string) (string, int, error) {
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return "", 0, ErrRPCAddress
	}
	port := cast.ToInt(portText)
	if port <= 0 {
		return "", 0, ErrRPCAddress
	}
	if configuredHost != "" {
		host = configuredHost
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return host, port, nil
}

func defaultOptions() options {
	var configCenter config.Config
	if path := novaflag.String("config"); path != "" {
		configCenter = config.New(config.WithSource(file.NewSource(path)))
	} else {
		configCenter = config.New()
	}
	return options{
		serviceName: novaflag.String("service"),
		instanceID:  novaflag.String("id"),
		rpcListen:   novaflag.String("rpc-listen", "127.0.0.1:0"),
		rpcHost:     novaflag.String("rpc-host"),
		weight:      novaflag.Int("weight", 1),
		config:      configCenter,
		shutdownTimeout: novaflag.Duration(
			"shutdown-timeout",
			defaultShutdownTimeout,
		),
	}
}

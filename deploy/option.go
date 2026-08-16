package deploy

import (
	"net"
	"time"

	"github.com/2comjie/nova/app"
	"github.com/2comjie/nova/app/gate"
	"github.com/2comjie/nova/app/node"
	"github.com/2comjie/nova/config"
	"github.com/2comjie/nova/locator"
	"github.com/2comjie/nova/network"
	"github.com/2comjie/nova/registry"
	rpcclient "github.com/2comjie/nova/rpc/client"
	"google.golang.org/grpc"
)

type options struct {
	serviceName string
	instanceID  string
	metaData    map[string]string
	weight      int

	config   config.Config
	registry registry.Registry
	discover registry.Discover
	locator  locator.Locator

	rpcListen        string
	rpcHost          string
	rpcListener      net.Listener
	rpcServer        *grpc.Server
	rpcClient        *rpcclient.Client
	rpcServerOptions []grpc.ServerOption
	rpcClientOptions []rpcclient.Option

	gateRouter     *gate.Router
	nodeRouter     *node.Router
	networkOptions []network.Option
	hooks          network.Hooks
	errorHandler   gate.ErrorHandler
	locatorTimeout time.Duration
	components     []app.Component

	shutdownTimeout time.Duration
}

type Option func(*options)

func WithServiceName(serviceName string) Option {
	return func(options *options) {
		options.serviceName = serviceName
	}
}

func WithInstanceID(instanceID string) Option {
	return func(options *options) {
		options.instanceID = instanceID
	}
}

func WithMetaData(metaData map[string]string) Option {
	return func(options *options) {
		options.metaData = metaData
	}
}

func WithWeight(weight int) Option {
	return func(options *options) {
		options.weight = weight
	}
}

func WithConfig(config config.Config) Option {
	return func(options *options) {
		options.config = config
	}
}

func WithRegistry(registry registry.Registry) Option {
	return func(options *options) {
		options.registry = registry
	}
}

func WithDiscover(discover registry.Discover) Option {
	return func(options *options) {
		options.discover = discover
	}
}

func WithLocator(locator locator.Locator) Option {
	return func(options *options) {
		options.locator = locator
	}
}

func WithRPCListen(address string) Option {
	return func(options *options) {
		options.rpcListen = address
	}
}

func WithRPCHost(host string) Option {
	return func(options *options) {
		options.rpcHost = host
	}
}

func WithRPCListener(listener net.Listener) Option {
	return func(options *options) {
		options.rpcListener = listener
	}
}

func WithRPCServer(server *grpc.Server) Option {
	return func(options *options) {
		options.rpcServer = server
	}
}

func WithRPCClient(client *rpcclient.Client) Option {
	return func(options *options) {
		options.rpcClient = client
	}
}

func WithRPCServerOptions(serverOptions ...grpc.ServerOption) Option {
	return func(options *options) {
		options.rpcServerOptions = append(options.rpcServerOptions, serverOptions...)
	}
}

func WithRPCClientOptions(clientOptions ...rpcclient.Option) Option {
	return func(options *options) {
		options.rpcClientOptions = append(options.rpcClientOptions, clientOptions...)
	}
}

func WithGateRouter(router *gate.Router) Option {
	return func(options *options) {
		options.gateRouter = router
	}
}

func WithNodeRouter(router *node.Router) Option {
	return func(options *options) {
		options.nodeRouter = router
	}
}

func WithNetworkOptions(networkOptions ...network.Option) Option {
	return func(options *options) {
		options.networkOptions = append(options.networkOptions, networkOptions...)
	}
}

func WithGateHooks(hooks network.Hooks) Option {
	return func(options *options) {
		options.hooks = hooks
	}
}

func WithGateErrorHandler(handler gate.ErrorHandler) Option {
	return func(options *options) {
		options.errorHandler = handler
	}
}

func WithGateLocatorTimeout(timeout time.Duration) Option {
	return func(options *options) {
		options.locatorTimeout = timeout
	}
}

func WithComponents(components ...app.Component) Option {
	return func(options *options) {
		options.components = append(options.components, components...)
	}
}

func WithShutdownTimeout(timeout time.Duration) Option {
	return func(options *options) {
		if timeout > 0 {
			options.shutdownTimeout = timeout
		}
	}
}

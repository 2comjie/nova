package deploy

import (
	"net"
	"time"

	"github.com/2comjie/wali/app"
	"github.com/2comjie/wali/app/gate"
	"github.com/2comjie/wali/app/node"
	"github.com/2comjie/wali/config"
	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/network"
	"github.com/2comjie/wali/registry"
	rpcclient "github.com/2comjie/wali/rpc/client"
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

// Option 修改Gate和Node的默认部署参数。
type Option func(*options)

// WithServiceName 设置服务名。
func WithServiceName(serviceName string) Option {
	return func(options *options) {
		options.serviceName = serviceName
	}
}

// WithInstanceID 设置当前进程内的服务实例ID。
func WithInstanceID(instanceID string) Option {
	return func(options *options) {
		options.instanceID = instanceID
	}
}

// WithMetaData 设置注册到服务发现的元数据。
func WithMetaData(metaData map[string]string) Option {
	return func(options *options) {
		options.metaData = metaData
	}
}

// WithWeight 设置负载均衡权重。
func WithWeight(weight int) Option {
	return func(options *options) {
		options.weight = weight
	}
}

// WithConfig 设置配置中心。
func WithConfig(config config.Config) Option {
	return func(options *options) {
		options.config = config
	}
}

// WithRegistry 设置服务注册器。
func WithRegistry(registry registry.Registry) Option {
	return func(options *options) {
		options.registry = registry
	}
}

// WithDiscover 设置服务发现器。
func WithDiscover(discover registry.Discover) Option {
	return func(options *options) {
		options.discover = discover
	}
}

// WithLocator 设置Gate和Node共用的定位存储。
func WithLocator(locator locator.Locator) Option {
	return func(options *options) {
		options.locator = locator
	}
}

// WithRPCListen 设置RPC监听地址，端口可以传0让系统自动分配。
func WithRPCListen(address string) Option {
	return func(options *options) {
		options.rpcListen = address
	}
}

// WithRPCHost 设置注册到服务发现中的RPC主机地址。
func WithRPCHost(host string) Option {
	return func(options *options) {
		options.rpcHost = host
	}
}

// WithRPCListener 使用已经创建的RPC Listener。
// Listener交给deploy管理，Shutdown后会被关闭。
func WithRPCListener(listener net.Listener) Option {
	return func(options *options) {
		options.rpcListener = listener
	}
}

// WithRPCServer 使用已经创建的gRPC Server。
func WithRPCServer(server *grpc.Server) Option {
	return func(options *options) {
		options.rpcServer = server
	}
}

// WithRPCClient 使用已经创建的Node间RPC Client。
// Client交给deploy管理，Shutdown后会被关闭。
func WithRPCClient(client *rpcclient.Client) Option {
	return func(options *options) {
		options.rpcClient = client
	}
}

// WithRPCServerOptions 设置默认gRPC Server的创建参数。
func WithRPCServerOptions(serverOptions ...grpc.ServerOption) Option {
	return func(options *options) {
		options.rpcServerOptions = append(options.rpcServerOptions, serverOptions...)
	}
}

// WithRPCClientOptions 设置Node间RPC Client参数和自定义Balancer。
func WithRPCClientOptions(clientOptions ...rpcclient.Option) Option {
	return func(options *options) {
		options.rpcClientOptions = append(options.rpcClientOptions, clientOptions...)
	}
}

// WithGateRouter 设置Gate Router。
func WithGateRouter(router *gate.Router) Option {
	return func(options *options) {
		options.gateRouter = router
	}
}

// WithNodeRouter 设置Node Router。
func WithNodeRouter(router *node.Router) Option {
	return func(options *options) {
		options.nodeRouter = router
	}
}

// WithNetworkOptions 设置Gate客户端网络服务参数。
func WithNetworkOptions(networkOptions ...network.Option) Option {
	return func(options *options) {
		options.networkOptions = append(options.networkOptions, networkOptions...)
	}
}

// WithGateHooks 设置Gate透传给network的生命周期钩子。
func WithGateHooks(hooks network.Hooks) Option {
	return func(options *options) {
		options.hooks = hooks
	}
}

// WithGateErrorHandler 设置Gate转发错误处理函数。
func WithGateErrorHandler(handler gate.ErrorHandler) Option {
	return func(options *options) {
		options.errorHandler = handler
	}
}

// WithGateLocatorTimeout 设置Gate更新玩家定位信息的超时时间。
func WithGateLocatorTimeout(timeout time.Duration) Option {
	return func(options *options) {
		options.locatorTimeout = timeout
	}
}

// WithComponents 添加由Node管理生命周期的业务组件。
func WithComponents(components ...app.Component) Option {
	return func(options *options) {
		options.components = append(options.components, components...)
	}
}

// WithShutdownTimeout 设置Run收到退出信号后的最大关闭时间。
func WithShutdownTimeout(timeout time.Duration) Option {
	return func(options *options) {
		if timeout > 0 {
			options.shutdownTimeout = timeout
		}
	}
}

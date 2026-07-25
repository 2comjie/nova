package client

import (
	"github.com/2comjie/wali/rpc/lx"
	"google.golang.org/grpc"
)

type options struct {
	dialOptions []grpc.DialOption
	balancers   map[lx.BalancePolicy]Balancer
}

// Option 设置 RPC Client。
type Option func(*options)

func defaultOptions() options {
	return options{
		balancers: map[lx.BalancePolicy]Balancer{
			lx.BalanceRoundRobin:         newRoundRobinBalancer(),
			lx.BalanceWeightedRoundRobin: newWeightedRoundRobinBalancer(),
		},
	}
}

// WithDialOptions 设置 gRPC 连接参数。
func WithDialOptions(opts ...grpc.DialOption) Option {
	return func(options *options) {
		options.dialOptions = append(options.dialOptions, opts...)
	}
}

// WithBalancer 注册或替换指定名称的负载均衡器。
func WithBalancer(name lx.BalancePolicy, balancer Balancer) Option {
	return func(options *options) {
		if name != "" && balancer != nil {
			options.balancers[name] = balancer
		}
	}
}

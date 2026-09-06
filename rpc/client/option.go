package client

import (
	"github.com/2comjie/nova/rpc"
	"github.com/2comjie/nova/rpc/lx"
)

type options struct {
	dialOptions []rpc.ConnOption
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

// WithConnOptions 设置 TCP RPC 连接参数。
func WithConnOptions(opts ...rpc.ConnOption) Option {
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

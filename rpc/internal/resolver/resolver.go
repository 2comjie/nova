package resolver

import (
	"google.golang.org/grpc/resolver"
)

const Scheme = "wali"

func init() {
	resolver.Register(&builder{})
}

type builder struct{}

func (b *builder) Build(target resolver.Target, cc resolver.ClientConn, opts resolver.BuildOptions) (resolver.Resolver, error) {
	r := &waliResolver{cc: cc}
	r.ResolveNow(resolver.ResolveNowOptions{})
	return r, nil
}

func (b *builder) Scheme() string {
	return Scheme
}

type waliResolver struct {
	cc resolver.ClientConn
}

func (r *waliResolver) ResolveNow(opts resolver.ResolveNowOptions) {
	// 给一个占位地址触发 gRPC 创建 Balancer/Picker
	r.cc.UpdateState(resolver.State{
		Endpoints: []resolver.Endpoint{{
			Addresses: []resolver.Address{{Addr: "0.0.0.0:0"}},
		}},
	})
}

func (r *waliResolver) Close() {}

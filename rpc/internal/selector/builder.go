package selector

import (
	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/registry"
	"google.golang.org/grpc/balancer"
)

const Name = "wali_selector"

type Builder struct {
	discover registry.Discover
	locator  locator.Locator
}

func NewBuilder(discover registry.Discover, locator locator.Locator) *Builder {
	return &Builder{
		discover: discover,
		locator:  locator,
	}
}

func RegisterBuilder(discover registry.Discover, locator locator.Locator) {
	balancer.Register(NewBuilder(discover, locator))
}

func (b *Builder) Build(cc balancer.ClientConn, opts balancer.BuildOptions) balancer.Balancer {
	return NewBalancer(b.discover, b.locator, cc)
}

func (b *Builder) Name() string {
	return Name
}

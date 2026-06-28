package client

import (
	"sync"

	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/registry"
	"github.com/2comjie/wali/rpc/internal/selector"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	_ "github.com/2comjie/wali/rpc/internal/resolver" // 注册 wali scheme
)

var once sync.Once

func Dial(discover registry.Discover, locator locator.Locator) (*grpc.ClientConn, error) {
	once.Do(func() {
		selector.RegisterBuilder(discover, locator)
	})

	return grpc.Dial("wali:///",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"`+selector.Name+`"}`),
	)
}

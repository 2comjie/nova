package rpcClient

import (
	"sync"

	"github.com/2comjie/wali/locator"
	"github.com/2comjie/wali/logx"
	"github.com/2comjie/wali/registry"
	"github.com/2comjie/wali/rpc/internal/selector"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	_ "github.com/2comjie/wali/rpc/internal/resolver" // 注册 wali scheme
)

var once sync.Once

func NewClient(discover registry.Discover, locator locator.Locator, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	once.Do(func() {
		selector.RegisterBuilder(discover, locator)
		logx.Infof("register rpc selector: %s", selector.Name)
	})
	options := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"` + selector.Name + `"}`),
	}
	options = append(options, opts...)
	client, err := grpc.NewClient("wali:///", options...)
	return client, err
}

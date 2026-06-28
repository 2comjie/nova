package main

import (
	"context"

	"github.com/2comjie/wali/core/util"
	"github.com/2comjie/wali/examples/external"
	"github.com/2comjie/wali/examples/rpcdemo/pbDemo"
	"github.com/2comjie/wali/locator"
	redisLocator "github.com/2comjie/wali/locator/redis"
	"github.com/2comjie/wali/logx"
	redisRegistry "github.com/2comjie/wali/registry/redis"
	rpcClient "github.com/2comjie/wali/rpc/client"
	"github.com/2comjie/wali/rpc/lx"
)

func main() {
	rdb := external.RedisClient()
	dis := redisRegistry.NewDiscover(rdb)
	defer dis.Close()
	loc := locator.NewNodeLocator(redisLocator.NewProvider(rdb))
	defer loc.Close()
	client, err := rpcClient.Dial(dis, loc)
	if err != nil {
		panic(err)
	}
	demoClient := pbDemo.NewHayServiceClient(client)

	// 1. 直连
	directCtx := lx.WithDirect(context.Background(), "127.0.0.1:8080")
	rsp, err := demoClient.SayHay(directCtx, &pbDemo.HayRequest{Name: "direct"})
	if err != nil {
		panic(err)
	} else {
		logx.Infof("direct: %s", rsp.Message)
	}
	// 2. 定位到具体的节点
	nodeCtx := lx.WithNode(context.Background(), "demo-1")
	rsp, err = demoClient.SayHay(nodeCtx, &pbDemo.HayRequest{Name: "node"})
	if err != nil {
		panic(err)
	} else {
		logx.Infof("node: %s", rsp.Message)
	}
	// 3. 负载均衡
	balanceCtx := lx.WithBalance(context.Background(), "demo")
	rsp, err = demoClient.SayHay(balanceCtx, &pbDemo.HayRequest{Name: "balance"})
	if err != nil {
		panic(err)
	}
	// 4. 节点路由
	// 先绑定下节点 再做
	err = loc.Bind(context.Background(), "demo", "uid-01", "demo-1")
	if err != nil {
		panic(err)
	}
	err = loc.Bind(context.Background(), "demo", "uid-02", "demo-2")
	if err != nil {
		panic(err)
	}
	selectCtx := lx.WithSelect(context.Background(), "demo", "uid-01")
	rsp, err = demoClient.SayHay(selectCtx, &pbDemo.HayRequest{Name: "select1"})
	if err != nil {
		panic(err)
	} else {
		logx.Infof("select: %s", rsp.Message)
	}
	selectCtx = lx.WithSelect(context.Background(), "demo", "uid-02")
	rsp, err = demoClient.SayHay(selectCtx, &pbDemo.HayRequest{Name: "select2"})
	if err != nil {
		panic(err)
	} else {
		logx.Infof("select: %s", rsp.Message)
	}
	util.WaitUntilSignaled()
}

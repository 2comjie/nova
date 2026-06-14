package main

import (
	"context"

	"github.com/2comjie/wali/core/util"
	"github.com/2comjie/wali/examples/internal/external"
	chatpb "github.com/2comjie/wali/examples/rpc/pb/proto/chat"
	redisLocator "github.com/2comjie/wali/locator/redis"
	"github.com/2comjie/wali/logx"
	redisRegistry "github.com/2comjie/wali/registry/redis"
	"github.com/2comjie/wali/rpc/client"
	"github.com/2comjie/wali/rpc/lx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	dis := redisRegistry.NewDiscover(external.RedisClient)
	defer dis.Close()
	locator := redisLocator.NewProvider(external.RedisClient)
	defer locator.Close()
	cl := client.NewClient(dis, locator, grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer cl.Close()

	chatClient := chatpb.NewChatServiceClient(cl)
	err := locator.Bind(context.Background(), "chat-service", "user-1", "chat-service-1")
	if err != nil {
		panic(err)
	}

	rsp, err := chatClient.Say(lx.WithRouteKey(context.Background(), "chat-service", "user-1"), &chatpb.SayRequest{Name: "wali"})
	if err != nil {
		panic(err)
	}
	logx.Infof("rpc client chatClient.Say() success: %v", rsp)
	util.WaitUntilSignaled()
}

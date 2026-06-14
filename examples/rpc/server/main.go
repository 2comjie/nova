package main

import (
	"context"
	"net"

	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/core/util"
	"github.com/2comjie/wali/examples/internal/external"
	chatpb "github.com/2comjie/wali/examples/rpc/pb/proto/chat"
	redisRegistry "github.com/2comjie/wali/registry/redis"
	"google.golang.org/grpc"
)

type chatServer struct {
	chatpb.UnimplementedChatServiceServer
}

func (s *chatServer) Say(ctx context.Context, req *chatpb.SayRequest) (*chatpb.SayResponse, error) {
	return &chatpb.SayResponse{
		Message: "hello" + req.GetName(),
	}, nil
}

func main() {
	register := redisRegistry.NewRegistry(external.RedisClient)
	serviceInstance := endpoint.ServiceInstance{
		ID:       "chat-service-1",
		MetaData: nil,
		Weight:   0,
		Status:   endpoint.Work,
		RpcServices: map[string]endpoint.RpcService{
			chatpb.ChatService_ServiceName: {
				Name: chatpb.ChatService_ServiceName,
			},
		},
		Routes:  nil,
		RpcHost: "127.0.0.1", // TODO 切换成内部的IP
		RpcPort: 6000,
	}
	err := register.Register(serviceInstance)
	defer func() {
		err = register.Deregister(serviceInstance.ID)
		register.Close()
	}()

	if err != nil {
		panic(err)
	}

	rpcServer := grpc.NewServer()
	chatpb.RegisterChatServiceServer(rpcServer, &chatServer{})
	listener, err := net.Listen("tcp", ":6000")
	if err != nil {
		panic(err)
	}
	err = rpcServer.Serve(listener)
	if err != nil {
		panic(err)
	}
	util.WaitUntilSignaled()
}

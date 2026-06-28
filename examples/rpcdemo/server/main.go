package main

import (
	"context"
	"fmt"
	"net"

	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/core/util"
	"github.com/2comjie/wali/examples/external"
	pb "github.com/2comjie/wali/examples/rpcdemo/pbDemo"
	redisRegistry "github.com/2comjie/wali/registry/redis"
	"google.golang.org/grpc"
)

type hayServer struct {
	pb.UnimplementedHayServiceServer
}

func (s *hayServer) SayHay(ctx context.Context, req *pb.HayRequest) (*pb.HayResponse, error) {
	fmt.Println("received:", req.Name)
	return &pb.HayResponse{Message: "Hay " + req.Name + "!"}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":8080")
	if err != nil {
		panic(err)
	}

	// 注册到 Redis
	reg := redisRegistry.NewRegistry(external.RedisClient())
	_ = reg.Register(endpoint.ServiceInstance{
		ID:     "server-01",
		Status: endpoint.Work,
		RpcServices: map[string]endpoint.RpcService{
			"demo": {Name: "demo", Desc: "demo service"},
		},
		RpcHost: "127.0.0.1",
		RpcPort: 8080,
	})
	defer reg.Deregister("server-01")

	s := grpc.NewServer()
	pb.RegisterHayServiceServer(s, &hayServer{})

	go func() {
		fmt.Println("demo server listening on :8080")
		s.Serve(lis)
	}()

	util.WaitUntilSignaled()
	fmt.Println("shutting down...")
	s.GracefulStop()
}

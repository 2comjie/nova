package main

import (
	"context"
	"fmt"
	"net"

	"github.com/2comjie/wali/core/endpoint"
	"github.com/2comjie/wali/core/util"
	"github.com/2comjie/wali/examples/external"
	pb "github.com/2comjie/wali/examples/rpcdemo/pbDemo"
	"github.com/2comjie/wali/flag"
	"github.com/2comjie/wali/logx"
	redisRegistry "github.com/2comjie/wali/registry/redis"
	"google.golang.org/grpc"
)

type hayServer struct {
	pb.UnimplementedHayServiceServer
}

func (s *hayServer) SayHay(ctx context.Context, req *pb.HayRequest) (*pb.HayResponse, error) {
	logx.Infof("received %s", req.Name)
	return &pb.HayResponse{Message: "Hay " + req.Name + "!"}, nil
}

func main() {
	insId := flag.String("insId", "demo-1")
	port := flag.Int("port", 8080)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		panic(err)
	}

	// 注册到 Redis
	reg := redisRegistry.NewRegistry(external.RedisClient())
	_ = reg.Register(endpoint.ServiceInstance{
		ID: insId,
		RpcServices: map[string]endpoint.RpcService{
			"demo": {Name: "demo", Desc: "demo service"},
		},
		RpcHost: "127.0.0.1",
		RpcPort: port,
	})
	defer reg.Deregister("server-01")

	s := grpc.NewServer()
	pb.RegisterHayServiceServer(s, &hayServer{})

	go func() {
		logx.Infof("demo server listening on :%d", port)
		s.Serve(lis)
	}()

	util.WaitUntilSignaled()
	fmt.Println("shutting down...")
	s.GracefulStop()
}

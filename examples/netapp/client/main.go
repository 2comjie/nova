package main

import (
	"context"

	"github.com/2comjie/wali/logx"
	"github.com/2comjie/wali/network"
	netTcp "github.com/2comjie/wali/network/transport/tcp"
)

func main() {
	dialer := netTcp.NewDialer("127.0.0.1:8080")
	netClient, err := network.NewClient(network.WithDialer(dialer))
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = netClient.Close()
	}()
	err = netClient.Dial(context.Background())
	if err != nil {
		panic(err)
	}

	err = netClient.Bind(context.Background(), []byte("demo-uid-01"))
	if err != nil {
		panic(err)
	}

	rsp, err := netClient.Call(context.Background(), 1, []byte("hello"))
	if err != nil {
		panic(err)
	}
	logx.Infof("rsp: %s", rsp)
}

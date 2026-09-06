package main

import (
	"context"
	"errors"
	"strconv"

	"github.com/2comjie/nova/core/util"
	"github.com/2comjie/nova/logx"
	"github.com/2comjie/nova/network"
	netTcp "github.com/2comjie/nova/network/transport/tcp"
)

func main() {
	listener, err := netTcp.Listen(":8080")
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = listener.Close()
	}()
	netServer := network.NewServer(
		network.WithListener(listener),
		network.WithAuther(network.AuthFunc(func(token []byte) (uid uint64, err error) {
			if len(token) == 0 {
				return 0, errors.New("token is empty")
			}
			return strconv.ParseUint(string(token), 10, 64)
		})),
		network.WithHooks(network.Hooks{
			OnSessionStart: func(session *network.Session) {
				logx.Infof("session start: %d", session.Id)
			},
			OnSessionEnd: func(_ context.Context, session *network.Session) {
				logx.Infof("session end: %d", session.Id)
			},
			OnSessionBind: func(session *network.Session) error {
				logx.Infof("session bind: %d %d", session.Id, session.Uid())
				return nil
			},
			OnHeartbeat: func(session *network.Session) {
				logx.Infof("session heartbeat: %d %d", session.Id, session.Uid())
			},
			OnReq: func(context *network.ReqContext) {
				logx.Infof("session req: %d %d %d %d %s", context.Session.Id, context.Session.Uid(), context.Request.Route, context.Request.Type, context.Request.Body)
				_ = context.Write([]byte("hello"))
			},
		}),
	)
	err = netServer.Start()
	if err != nil {
		panic(err)
	}
	util.WaitUntilSignaled()
	_ = netServer.Shutdown(context.Background())
	logx.Info("server shutdown")
}

package main

import (
	"context"
	"errors"
	"strings"

	"github.com/2comjie/wali/core/util"
	"github.com/2comjie/wali/logx"
	"github.com/2comjie/wali/network"
	netTcp "github.com/2comjie/wali/network/transport/tcp"
)

func main() {
	listener, err := netTcp.Listen(":8080")
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = listener.Close()
	}()
	netServer, err := network.NewServer(
		network.WithListener(listener),
		network.WithAuther(network.AuthFunc(func(token []byte) (uid string, err error) {
			tokenStr := strings.TrimSpace(string(token))
			if tokenStr == "" {
				return "", errors.New("token is empty")
			}
			return tokenStr, nil
		})),
		network.WithHooks(network.Hooks{
			OnSessionStart: func(session *network.Session) {
				logx.Infof("session start: %d", session.ID)
			},
			OnSessionEnd: func(session *network.Session) {
				logx.Infof("session end: %d", session.ID)
			},
			OnSessionBind: func(session *network.Session) error {
				logx.Infof("session bind: %d %s", session.ID, session.UID())
				return nil
			},
			OnHeartbeat: func(session *network.Session) {
				logx.Infof("session heartbeat: %d %s", session.ID, session.UID())
			},
			OnReq: func(context *network.ReqContext) {
				logx.Infof("session req: %d %s %d %d %s", context.Session.ID, context.Session.UID(), context.Request.Route, context.Request.Type, context.Request.Body)
				_ = context.Write([]byte("hello"))
			},
		}),
	)
	if err != nil {
		panic(err)
	}

	err = netServer.Start()
	if err != nil {
		panic(err)
	}
	util.WaitUntilSignaled()
	_ = netServer.Shutdown(context.Background())
	logx.Info("server shutdown")
}

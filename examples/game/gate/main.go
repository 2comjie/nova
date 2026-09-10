package main

import (
	"context"
	"errors"
	"strings"

	"github.com/2comjie/nova/config"
	"github.com/2comjie/nova/config/file"
	"github.com/2comjie/nova/deploy"
	"github.com/2comjie/nova/examples/game/shared"
	"github.com/2comjie/nova/flag"
	"github.com/2comjie/nova/locator"
	"github.com/2comjie/nova/logx"
	"github.com/2comjie/nova/network"
	nettcp "github.com/2comjie/nova/network/transport/tcp"
	"github.com/spf13/cast"
)

func main() {
	infrastructure := shared.NewInfrastructure(
		flag.String("redis", "127.0.0.1:6379"),
	)
	defer infrastructure.Redis.Close()

	clientListener, err := nettcp.Listen(
		flag.String("listen", "127.0.0.1:8000"),
	)
	if err != nil {
		panic(err)
	}
	configCenter := config.New(config.WithSource(file.NewSource(
		flag.String("config", "./game/config/gate.yml"),
	)))

	options := infrastructure.DeployOptions()
	options = append(options,
		deploy.WithServiceName(flag.String("service", locator.GateName)),
		deploy.WithInstanceId(flag.String("id", "gate-1")),
		deploy.WithConfig(configCenter),
		deploy.WithNetworkOptions(
			network.WithListener(clientListener),
			network.WithAuther(network.AuthFunc(func(token []byte) (uint64, error) {
				if len(token) == 0 {
					return 0, errors.New("token不能为空")
				}
				// token 是十进制 Uid，不能让前导 0 被 cast 当成八进制。
				for _, digit := range token {
					if digit < '0' || digit > '9' {
						return 0, errors.New("token中的Uid无效")
					}
				}
				uid, err := cast.ToUint64E(strings.TrimLeft(string(token), "0"))
				if err != nil {
					return 0, errors.New("token中的Uid无效")
				}
				return uid, nil
			})),
		),
		deploy.WithGateHooks(network.Hooks{
			OnSessionBind: func(session *network.Session) error {
				logx.Infof("玩家连接 uid=%d session=%d", session.Uid(), session.Id)
				return nil
			},
			OnSessionEnd: func(_ context.Context, session *network.Session) {
				logx.Infof("玩家断开 uid=%d session=%d", session.Uid(), session.Id)
			},
		}),
	)

	gate, err := deploy.Gate(options...)
	if err != nil {
		panic(err)
	}
	logx.Infof(
		"Gate启动 id=%s client=%s rpc=%s",
		gate.Instance().Id,
		clientListener.Addr(),
		gate.Instance().RpcTarget(),
	)
	if err := gate.Run(); err != nil {
		panic(err)
	}
}

package main

import (
	"errors"

	"github.com/2comjie/nova/config"
	"github.com/2comjie/nova/config/file"
	"github.com/2comjie/nova/deploy"
	"github.com/2comjie/nova/examples/game/shared"
	"github.com/2comjie/nova/flag"
	"github.com/2comjie/nova/locator"
	"github.com/2comjie/nova/logx"
	"github.com/2comjie/nova/network"
	nettcp "github.com/2comjie/nova/network/transport/tcp"
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
		deploy.WithInstanceID(flag.String("id", "gate-1")),
		deploy.WithConfig(configCenter),
		deploy.WithNetworkOptions(
			network.WithListener(clientListener),
			network.WithAuther(network.AuthFunc(func(token []byte) (string, error) {
				if len(token) == 0 {
					return "", errors.New("token不能为空")
				}
				// 演示环境直接把token作为UID，生产环境应校验并解析正式token。
				return string(token), nil
			})),
		),
		deploy.WithGateHooks(network.Hooks{
			OnSessionBind: func(session *network.Session) error {
				logx.Infof("玩家连接 uid=%s session=%d", session.UID(), session.ID)
				return nil
			},
			OnSessionEnd: func(session *network.Session) {
				logx.Infof("玩家断开 uid=%s session=%d", session.UID(), session.ID)
			},
		}),
	)

	gate, err := deploy.Gate(options...)
	if err != nil {
		panic(err)
	}
	logx.Infof(
		"Gate启动 id=%s client=%s rpc=%s",
		gate.Instance().ID,
		clientListener.Addr(),
		gate.Instance().RpcTarget(),
	)
	if err := gate.Run(); err != nil {
		panic(err)
	}
}

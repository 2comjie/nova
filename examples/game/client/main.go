package main

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/2comjie/nova/core/util"
	"github.com/2comjie/nova/examples/game/shared"
	"github.com/2comjie/nova/flag"
	"github.com/2comjie/nova/logx"
	"github.com/2comjie/nova/network"
	nettcp "github.com/2comjie/nova/network/transport/tcp"
)

func main() {
	uid := flag.Uint64("uid", 1)
	client, err := network.NewClient(network.WithDialer(
		nettcp.NewDialer(flag.String("addr", "127.0.0.1:8000")),
	))
	if err != nil {
		panic(err)
	}
	defer client.Close()

	client.OnPush(shared.RouteChatPush, func(_ context.Context, body []byte) {
		var push shared.ChatPush
		if err := json.Unmarshal(body, &push); err != nil {
			logx.Errorf("聊天Push格式错误: %v", err)
			return
		}
		logx.Infof("收到聊天 from=%d text=%s", push.FromUID, push.Text)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Dial(ctx); err != nil {
		panic(err)
	}
	if err := client.Bind(ctx, []byte(strconv.FormatUint(uid, 10))); err != nil {
		panic(err)
	}
	logx.Infof("客户端绑定完成 uid=%d", uid)

	callPlayer(ctx, client, shared.RoutePlayerGet, nil, "当前养成数据")
	addExpBody, err := json.Marshal(shared.PlayerAddExpRequest{
		Exp: flag.Int("exp", 120),
	})
	if err != nil {
		panic(err)
	}
	callPlayer(ctx, client, shared.RoutePlayerAddExp, addExpBody, "增加经验后")

	toUID := flag.Uint64("to")
	if toUID != 0 {
		sendChat(ctx, client, toUID, flag.String("message", "你好"))
	}

	logx.Infof("客户端保持在线，按Ctrl+C退出")
	util.WaitUntilSignaled()
}

func callPlayer(
	ctx context.Context,
	client *network.Client,
	route uint32,
	body []byte,
	title string,
) {
	responseBody, err := client.Call(ctx, route, body)
	if err != nil {
		panic(err)
	}
	var profile shared.PlayerProfile
	if err := json.Unmarshal(responseBody, &profile); err != nil {
		panic(err)
	}
	logx.Infof(
		"%s uid=%d level=%d exp=%d gold=%d",
		title,
		profile.UID,
		profile.Level,
		profile.Exp,
		profile.Gold,
	)
}

func sendChat(ctx context.Context, client *network.Client, toUID uint64, text string) {
	body, err := json.Marshal(shared.ChatSendRequest{
		ToUID: toUID,
		Text:  text,
	})
	if err != nil {
		panic(err)
	}
	responseBody, err := client.Call(ctx, shared.RouteChatSend, body)
	if err != nil {
		panic(err)
	}
	var response shared.ChatSendResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		panic(err)
	}
	logx.Infof("聊天发送结果 success=%v error=%s", response.Success, response.Error)
}

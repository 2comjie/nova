package main

import (
	"encoding/json"

	"github.com/2comjie/nova/app/node"
	"github.com/2comjie/nova/deploy"
	"github.com/2comjie/nova/examples/game/shared"
	"github.com/2comjie/nova/flag"
	"github.com/2comjie/nova/logx"
)

func main() {
	infrastructure := shared.NewInfrastructure(
		flag.String("redis", "127.0.0.1:6379"),
	)
	defer infrastructure.Redis.Close()

	router := node.NewRouter()
	router.Handle(shared.RouteChatSend, onChatSend)

	options := infrastructure.DeployOptions()
	options = append(options,
		deploy.WithServiceName(flag.String("service", "chat")),
		deploy.WithInstanceID(flag.String("id", "chat-1")),
		deploy.WithNodeRouter(router),
	)
	chat, err := deploy.Node(options...)
	if err != nil {
		panic(err)
	}
	logx.Infof("Chat Node启动 id=%s rpc=%s", chat.Instance().ID, chat.Instance().RpcTarget())
	if err := chat.Run(); err != nil {
		panic(err)
	}
}

func onChatSend(ctx *node.Context) error {
	var request shared.ChatSendRequest
	if err := json.Unmarshal(ctx.Request.Body, &request); err != nil {
		return replyChat(ctx, shared.ChatSendResponse{Error: "聊天请求格式错误"})
	}
	pushBody, err := json.Marshal(shared.ChatPush{
		FromUID: ctx.Request.UID,
		Text:    request.Text,
	})
	if err != nil {
		return replyChat(ctx, shared.ChatSendResponse{Error: err.Error()})
	}
	if err := ctx.App.Push(ctx, request.ToUID, shared.RouteChatPush, pushBody); err != nil {
		return replyChat(ctx, shared.ChatSendResponse{Error: err.Error()})
	}
	logx.Infof("聊天发送 from=%d to=%d text=%s", ctx.Request.UID, request.ToUID, request.Text)
	return replyChat(ctx, shared.ChatSendResponse{Success: true})
}

func replyChat(ctx *node.Context, response shared.ChatSendResponse) error {
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	return ctx.Reply(body)
}

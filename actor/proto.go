package actor

import (
	"context"
	"reflect"

	"github.com/2comjie/nova/actor/actorDef"
	"github.com/2comjie/nova/app/node"
	"google.golang.org/protobuf/proto"
)

func (g *RouteGroup[T]) Reg[Req proto.Message, Rsp proto.Message](route uint32, handler func(actorValue T, pid actorDef.Pid, ctx *node.Context, req Req, rsp Rsp) error) {
	reqType := reflect.TypeFor[Req]().Elem()
	rspType := reflect.TypeFor[Rsp]().Elem()

	g.Handle(route, func(actorValue T, pid actorDef.Pid, ctx *node.Context) error {
		req := reflect.New(reqType).Interface().(Req)
		if err := proto.Unmarshal(ctx.Request.Body, req); err != nil {
			return err
		}

		rsp := reflect.New(rspType).Interface().(Rsp)
		if err := handler(actorValue, pid, ctx, req, rsp); err != nil {
			return err
		}
		if !ctx.NeedReply() {
			return nil
		}

		body, err := proto.Marshal(rsp)
		if err != nil {
			return err
		}
		return ctx.Reply(body)
	})
}

func (g *RPCRouteGroup[T]) Reg[Req proto.Message, Rsp proto.Message](route uint32, handler func(actorValue T, pid actorDef.Pid, ctx context.Context, req Req, rsp Rsp) error) {
	reqType := reflect.TypeFor[Req]().Elem()
	rspType := reflect.TypeFor[Rsp]().Elem()

	g.Handle(route, func(actorValue T, pid actorDef.Pid, ctx context.Context, message Message) ([]byte, error) {
		req := reflect.New(reqType).Interface().(Req)
		if err := proto.Unmarshal(message.Body, req); err != nil {
			return nil, err
		}

		rsp := reflect.New(rspType).Interface().(Rsp)
		if err := handler(actorValue, pid, ctx, req, rsp); err != nil {
			return nil, err
		}
		return proto.Marshal(rsp)
	})
}

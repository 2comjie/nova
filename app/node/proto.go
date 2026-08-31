package node

import (
	"reflect"

	"google.golang.org/protobuf/proto"
)

func (r *Router) Reg[Req proto.Message, Rsp proto.Message](route uint32, handler func(ctx *Context, req Req, rsp Rsp) error) {
	reqType := reflect.TypeFor[Req]().Elem()
	rspType := reflect.TypeFor[Rsp]().Elem()

	r.Handle(route, func(ctx *Context) error {
		req := reflect.New(reqType).Interface().(Req)
		if err := proto.Unmarshal(ctx.Request.Body, req); err != nil {
			return err
		}

		rsp := reflect.New(rspType).Interface().(Rsp)
		if err := handler(ctx, req, rsp); err != nil {
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

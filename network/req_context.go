package network

import (
	"context"
	"sync/atomic"

	"github.com/2comjie/nova/packet"
)

type ReqContext struct {
	context.Context
	Session   *Session
	Request   *packet.Message
	NeedReply bool

	options options
	written atomic.Bool
}

func (c *ReqContext) Write(body []byte) error {
	if !c.NeedReply {
		// 不用写返回值
		return nil
	}
	body, err := encodeBody(
		c.options,
		packet.Rsp,
		c.Request.Route,
		c.Request.Seq,
		body,
	)
	if err != nil {
		return err
	}
	if !c.written.CompareAndSwap(false, true) {
		return ErrResponseWritten
	}
	// Response delivery follows the connection lifetime, including custom
	// error responses produced after the business deadline has expired.
	err = c.Session.Conn.WriteContext(c.Session.ctx, &packet.Message{
		Type:  packet.Rsp,
		Route: c.Request.Route,
		Seq:   c.Request.Seq,
		Body:  body,
	})
	if err != nil {
		_ = c.Session.Conn.Close()
	}
	return err
}

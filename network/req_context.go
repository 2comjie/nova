package network

import (
	"sync/atomic"

	"github.com/2comjie/wali/packet"
)

type ReqContext struct {
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
	if !c.written.CompareAndSwap(false, true) {
		return ErrResponseWritten
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
	err = c.Session.Conn.Write(&packet.Message{
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

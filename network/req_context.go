package network

import (
	"sync/atomic"

	"github.com/2comjie/wali/packet"
)

// ReqContext 是 OnReq 收到的请求上下文。
//
// Request 的 Body 已完成解密和解压，只在 OnReq 执行期间有效。
// Request.Seq 为零表示 Tell，不允许返回 Rsp。
// 业务需要响应时调用 Write；不调用 Write 就不会发送 Rsp。
type ReqContext struct {
	Session   *Session
	Request   *packet.Message
	NeedReply bool

	options options
	written atomic.Bool
}

// Write 返回与当前 Req 使用相同 Route、Seq 的 Rsp，每个请求最多调用一次。
func (c *ReqContext) Write(body []byte) error {
	if !c.NeedReply {
		return ErrTellNoResponse
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

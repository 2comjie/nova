package gate

import (
	"context"
	"errors"

	"github.com/2comjie/wali/network"
)

var (
	ErrReplyNotAllowed = errors.New("gate: Tell请求不允许响应")
	ErrAlreadyReplied  = errors.New("gate: 请求已经响应")
)

// Context 是 Gate Filter 和转发逻辑共用的请求上下文。
type Context struct {
	context.Context

	App     *Proxy
	Session *network.Session
	Route   uint32
	Seq     uint64
	Body    []byte

	RouteID string
	Target  Target

	BindingKey      string
	NodeServiceName string
	NodeInstanceID  string

	needReply    bool
	replied      bool
	responseBody []byte
	forward      Handler
}

// NeedReply 表示客户端是否通过Call请求响应。
func (c *Context) NeedReply() bool {
	return c.needReply
}

// Reply 设置返回给客户端的数据，Tell请求和重复响应会被拒绝。
func (c *Context) Reply(body []byte) error {
	if !c.needReply {
		return ErrReplyNotAllowed
	}
	if c.replied {
		return ErrAlreadyReplied
	}
	c.replied = true
	c.responseBody = body
	return nil
}

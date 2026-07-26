package node

import (
	"context"
	"errors"
)

var (
	ErrReplyNotAllowed = errors.New("node: Tell请求不允许响应")
	ErrAlreadyReplied  = errors.New("node: 请求已经响应")
)

// Request 是 Node Router 当前处理的请求。
// Body 只在当前请求处理期间有效，异步使用时业务层必须自行复制。
type Request struct {
	Route           uint32
	UID             string
	Body            []byte
	GateServiceName string
	GateInstanceID  string
	NeedReply       bool
}

// Context 是 Node Handler 和 Middleware 共用的请求上下文。
type Context struct {
	context.Context

	App     *Proxy
	Request *Request

	replied      bool
	responseBody []byte
}

// NeedReply 表示当前请求是否允许响应客户端。
func (c *Context) NeedReply() bool {
	return c.Request.NeedReply
}

// Reply 设置返回给客户端的数据，Tell请求和重复响应会被拒绝。
func (c *Context) Reply(body []byte) error {
	if !c.Request.NeedReply {
		return ErrReplyNotAllowed
	}
	if c.replied {
		return ErrAlreadyReplied
	}
	c.replied = true
	c.responseBody = body
	return nil
}

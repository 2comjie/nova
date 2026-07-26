package gate

import (
	"context"

	"github.com/2comjie/wali/network"
)

// Context 是 Gate Filter 和转发逻辑共用的请求上下文。
type Context struct {
	context.Context

	Session *network.Session
	Route   uint32
	Seq     uint64
	Body    []byte

	RouteID string
	Target  Target

	Replied         bool
	ResponseBody    []byte
	NodeServiceName string
	NodeInstanceID  string

	needReply bool
	forward   Handler
}

// NeedReply 表示客户端是否通过Call请求响应。
func (c *Context) NeedReply() bool {
	return c != nil && c.needReply
}

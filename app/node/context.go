package node

import "context"

// Request 是 Node Router 当前处理的请求。
// 后续由 Node RPC 层补充来源网关、Session、UID、Seq 和 Body。
type Request struct {
	Route uint32
}

// Context 是 Node Handler 和 Middleware 共用的请求上下文。
type Context struct {
	context.Context
	Request *Request
}

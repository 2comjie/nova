package node

import (
	"context"
	"errors"
)

var (
	ErrReplyNotAllowed = errors.New("node: Tell请求不允许响应")
	ErrAlreadyReplied  = errors.New("node: 请求已经响应")
)

type Request struct {
	Route           uint32
	UID             uint64
	Body            []byte
	GateServiceName string
	GateInstanceID  string
	ActorKey        string
	NeedReply       bool
}

type Context struct {
	context.Context

	App     *Node
	Request *Request

	replied      bool
	responseBody []byte
}

func (c *Context) NeedReply() bool {
	return c.Request.NeedReply
}

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

func (c *Context) ResponseBody() []byte {
	return c.responseBody
}

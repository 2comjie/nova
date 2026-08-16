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

type Context struct {
	context.Context

	App     *Gate
	Session *network.Session
	Uid     string
	Route   uint32
	Seq     uint64
	Body    []byte

	RouteID  string
	Target   Target
	ActorKey string

	BindingKey      string
	NodeServiceName string
	NodeInstanceID  string

	needReply        bool
	replied          bool
	responseBody     []byte
	forward          Handler
	actorKeyResolver ActorKeyResolver
}

func (c *Context) NeedReply() bool {
	return c.needReply
}

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

package transport

import (
	"context"
	"errors"
	"net"

	"github.com/2comjie/nova/packet"
)

type Type string

const (
	TypeTCP Type = "tcp"
	TypeKCP Type = "kcp"
	TypeWS  Type = "ws"
)

var (
	ErrClosed         = errors.New("network: 连接已关闭")
	ErrStarted        = errors.New("network: 连接已经启动")
	ErrWriteQueueFull = errors.New("network: 写队列已满")
)

type Handler interface {
	HandleMessage(Conn, *packet.Message)
	HandleClose(Conn)
}

type Conn interface {
	Start(Handler) error
	Write(*packet.Message) error
	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	Type() Type
	Secure() bool
}

type Listener interface {
	Accept() (Conn, error)
	Close() error
	Addr() net.Addr
}

type Dialer interface {
	DialContext(context.Context) (Conn, error)
}

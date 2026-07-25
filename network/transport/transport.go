package transport

import (
	"context"
	"errors"
	"net"

	"github.com/2comjie/wali/packet"
)

// Type 表示底层传输类型。
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

// Handler 接收连接读取到的完整网络包及关闭事件。
type Handler interface {
	HandleMessage(Conn, *packet.Message)
	HandleClose(Conn)
}

// Conn 是 TCP、KCP 和 WebSocket 对 network 暴露的统一连接。
type Conn interface {
	Start(Handler) error
	Write(*packet.Message) error
	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	Type() Type
	Secure() bool
}

// Listener 接受底层连接。
type Listener interface {
	Accept() (Conn, error)
	Close() error
	Addr() net.Addr
}

// Dialer 建立一个底层连接。
type Dialer interface {
	DialContext(context.Context) (Conn, error)
}

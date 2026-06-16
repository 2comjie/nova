package network

import (
	"io"
	"net"
	"time"
)

type Transport interface {
	io.Reader
	io.Writer
	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
	SetWriteDeadline(t time.Time) error
}

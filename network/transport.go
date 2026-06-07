package network

import (
	"io"
	"net"
)

type Transport interface {
	io.Reader
	io.Writer
	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

package network

import (
	"io"
	"net"
)

type Transport interface {
	io.ReadWriteCloser
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

type Listener interface {
	io.Closer
	Listen(address string) error
	Addr() net.Addr
	Accept() (Transport, error)
	Protocol() string
}

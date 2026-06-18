package client

import (
	"io"
	"net"
)

type Transport interface {
	io.ReadWriteCloser
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

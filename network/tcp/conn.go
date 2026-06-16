package tcp

import (
	"net"
	"time"
)

type tcpTransport struct {
	conn net.Conn
}

func (t *tcpTransport) Read(p []byte) (n int, err error) {
	return t.conn.Read(p)
}

func (t *tcpTransport) Write(p []byte) (n int, err error) {
	return t.conn.Write(p)
}

func (t *tcpTransport) Close() error {
	return t.conn.Close()
}

func (t *tcpTransport) LocalAddr() net.Addr {
	return t.conn.LocalAddr()
}

func (t *tcpTransport) RemoteAddr() net.Addr {
	return t.conn.RemoteAddr()
}

func (tr *tcpTransport) SetWriteDeadline(deadline time.Time) error {
	return tr.conn.SetWriteDeadline(deadline)
}

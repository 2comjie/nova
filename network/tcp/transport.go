package tcp

import "net"

type transport struct {
	rawConn net.Conn
}

func (t *transport) Read(p []byte) (n int, err error) {
	return t.rawConn.Read(p)
}

func (t *transport) Write(p []byte) (n int, err error) {
	return t.rawConn.Write(p)
}

func (t *transport) Close() error {
	return t.rawConn.Close()
}

func (t *transport) LocalAddr() net.Addr {
	return t.rawConn.LocalAddr()
}

func (t *transport) RemoteAddr() net.Addr {
	return t.rawConn.RemoteAddr()
}

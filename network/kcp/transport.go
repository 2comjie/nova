package kcp

import (
	"net"

	kcp "github.com/xtaci/kcp-go"
)

type transport struct {
	sess *kcp.UDPSession
}

func newTransport(sess *kcp.UDPSession) *transport {
	return &transport{sess: sess}
}

func (t *transport) Read(p []byte) (n int, err error)  { return t.sess.Read(p) }
func (t *transport) Write(p []byte) (n int, err error) { return t.sess.Write(p) }
func (t *transport) Close() error                      { return t.sess.Close() }
func (t *transport) LocalAddr() net.Addr               { return t.sess.LocalAddr() }
func (t *transport) RemoteAddr() net.Addr              { return t.sess.RemoteAddr() }

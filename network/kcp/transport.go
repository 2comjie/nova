package kcp

import (
	"net"

	kcp "github.com/xtaci/kcp-go"
)

// kcpTransport 把 kcp.UDPSession 包装成 network.Transport
type kcpTransport struct {
	sess *kcp.UDPSession
}

func (t *kcpTransport) Read(p []byte) (int, error)  { return t.sess.Read(p) }
func (t *kcpTransport) Write(p []byte) (int, error) { return t.sess.Write(p) }
func (t *kcpTransport) Close() error                { return t.sess.Close() }
func (t *kcpTransport) LocalAddr() net.Addr         { return t.sess.LocalAddr() }
func (t *kcpTransport) RemoteAddr() net.Addr        { return t.sess.RemoteAddr() }

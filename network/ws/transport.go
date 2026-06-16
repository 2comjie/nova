package ws

import (
	"context"
	"io"
	"net"
	"time"

	"nhooyr.io/websocket"
)

// wsTransport 把 websocket.Conn 包装成 network.Transport
type wsTransport struct {
	conn *websocket.Conn
	ctx  context.Context
	r    io.Reader
}

func newTransport(ctx context.Context, conn *websocket.Conn) *wsTransport {
	return &wsTransport{conn: conn, ctx: ctx}
}

func (t *wsTransport) Read(p []byte) (int, error) {
	if t.r != nil {
		n, err := t.r.Read(p)
		if err == io.EOF {
			t.r = nil
			err = nil
		}
		return n, err
	}
	_, r, err := t.conn.Reader(t.ctx)
	if err != nil {
		return 0, err
	}
	t.r = r
	return t.r.Read(p)
}

func (t *wsTransport) Write(p []byte) (int, error) {
	err := t.conn.Write(t.ctx, websocket.MessageBinary, p)
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (t *wsTransport) Close() error {
	return t.conn.Close(websocket.StatusNormalClosure, "")
}

func (t *wsTransport) LocalAddr() net.Addr  { return addr("ws-local") }
func (t *wsTransport) RemoteAddr() net.Addr { return addr("ws-remote") }

// ws 写操作通过 ctx 控制超时，SetWriteDeadline 为空操作
func (t *wsTransport) SetWriteDeadline(_ time.Time) error { return nil }

// ws 没有标准 net.Addr，用简单包装
type addr string

func (a addr) Network() string { return "ws" }
func (a addr) String() string  { return string(a) }

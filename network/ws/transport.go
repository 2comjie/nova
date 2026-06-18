package ws

import (
	"io"
	"net"

	"github.com/gorilla/websocket"
)

// transport 将 gorilla/websocket.Conn 包装为 network.Transport
// gorilla/websocket 是消息协议，transport 负责将其适配为流式 IO
type transport struct {
	conn *websocket.Conn
	r    io.Reader // 当前消息的未读完部分
}

func newTransport(conn *websocket.Conn) *transport {
	return &transport{conn: conn}
}

// Read 从当前消息中读取数据；若当前消息已读完则读取下一条消息
func (t *transport) Read(p []byte) (n int, err error) {
	if t.r != nil {
		n, err = t.r.Read(p)
		if err == io.EOF {
			t.r = nil
			return n, nil // 消息边界，不向上层报错
		}
		return n, err
	}
	_, r, err := t.conn.NextReader()
	if err != nil {
		return 0, err
	}
	t.r = r
	return t.r.Read(p)
}

// Write 以二进制消息形式写入数据
func (t *transport) Write(p []byte) (n int, err error) {
	w, err := t.conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return 0, err
	}
	defer w.Close()

	return w.Write(p)
}

// Close 关闭 WebSocket 连接
func (t *transport) Close() error {
	return t.conn.Close()
}

func (t *transport) LocalAddr() net.Addr  { return t.conn.LocalAddr() }
func (t *transport) RemoteAddr() net.Addr { return t.conn.RemoteAddr() }

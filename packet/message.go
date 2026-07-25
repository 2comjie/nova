package packet

import "github.com/2comjie/wali/core/buffer"

// Type 表示网络包类型。
type Type uint16

const (
	Req     Type = 1
	Rsp     Type = 2
	Push    Type = 3
	Ping    Type = 4
	Pong    Type = 5
	BindReq Type = 6
	BindRsp Type = 7
)

// Message 是业务层可见的网络包。
//
// 从 Codec.Read 读取的 Message 由内存池持有，处理完成后必须调用 Release。
type Message struct {
	Type  Type
	Route uint32
	Seq   uint64
	Body  []byte

	buf *buffer.Bytes
}

// Release 归还读取 Message 时使用的内存。
func (m *Message) Release() {
	if m == nil || m.buf == nil {
		return
	}
	m.buf.Release()
	m.buf = nil
	m.Body = nil
}

package packet

import (
	"encoding/binary"

	"github.com/2comjie/wali/core/buffer"
	"github.com/2comjie/wali/core/util"
)

var ByteOrder binary.ByteOrder = binary.BigEndian

// [size(4)][head(4)][route(4)][seq(4)][data]
// head 位域:
//
//	bit 0-2  MessageType
//	bit 3-7  扩展标志
//	bit 8-15 保留
//	bit 16-31 保留
type MessageType int

const (
	Req  MessageType = 0
	Rsp  MessageType = 1
	Push MessageType = 2
	Ping MessageType = 3
	Pong MessageType = 4
)

const (
	bitTypePos = 0
	bitTypeLen = 3
	headerSize = 12 // head(4) + route(4) + seq(4)
)

type Message struct {
	buff buffer.Buffer
}

func (m *Message) MessageType() MessageType {
	head := ByteOrder.Uint32(m.buff.Bytes()[0:4])
	return MessageType(util.GetBits(byte(head), bitTypePos, bitTypeLen))
}

func (m *Message) Route() int32 {
	return int32(ByteOrder.Uint32(m.buff.Bytes()[4:8]))
}

func (m *Message) Seq() int32 {
	return int32(ByteOrder.Uint32(m.buff.Bytes()[8:12]))
}

func (m *Message) Data() []byte {
	return m.buff.Bytes()[headerSize:]
}

func (m *Message) Release() {
	m.buff.Release()
}

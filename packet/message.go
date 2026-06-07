package packet

import (
	"encoding/binary"

	"github.com/2comjie/wali/core/buffer"
	"github.com/2comjie/wali/core/util"
)

var ByteOrder binary.ByteOrder = binary.BigEndian

// head(1)+route(4)+seq(4)+data
type MessageType int

const (
	Req  MessageType = 0
	Rsp  MessageType = 1
	Push MessageType = 2
	Ping MessageType = 3
)

const (
	bitTypePos = 0
	bitTypeLen = 2
	headerSize = 9 // head(1) + route(4) + seq(4)
)

type Message struct {
	buff buffer.Buffer
}

func (m *Message) MessageType() MessageType {
	return MessageType(util.GetBits(m.buff.Bytes()[0], bitTypePos, bitTypeLen))
}

func (m *Message) Route() int32 {
	return int32(ByteOrder.Uint32(m.buff.Bytes()[1:5]))
}

func (m *Message) Seq() int32 {
	return int32(ByteOrder.Uint32(m.buff.Bytes()[5:9]))
}

func (m *Message) Data() []byte {
	return m.buff.Bytes()[headerSize:]
}

func (m *Message) Release() {
	m.buff.Release()
}

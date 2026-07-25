package packet

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/2comjie/wali/core/buffer"
)

const (
	Magic           uint16 = 0x822f
	HeaderSize             = 20
	DefaultMaxFrame        = 4 << 20 // 4MB
	DefaultMaxBind         = 16 << 10
	PingBodySize           = 8
)

var (
	ErrMagic       = errors.New("packet: magic错误")
	ErrFrameLength = errors.New("packet: 帧长度错误")
	ErrType        = errors.New("packet: 包类型错误")
	ErrRoute       = errors.New("packet: route错误")
	ErrSeq         = errors.New("packet: seq错误")
	ErrBody        = errors.New("packet: body错误")
)

// Codec 负责固定包头的编码、解码和长度限制
type Codec struct {
	maxFrame uint32
	maxBind  uint32
}

// NewCodec 创建包编解码器。
func NewCodec(maxFrame uint32) *Codec {
	if maxFrame < HeaderSize {
		maxFrame = DefaultMaxFrame
	}
	return &Codec{
		maxFrame: maxFrame,
		maxBind:  DefaultMaxBind,
	}
}

// Read 从流中读取一个完整包。返回的 Message 使用完后必须 Release。
func (c *Codec) Read(r io.Reader) (*Message, error) {
	header := buffer.MallocBytes(HeaderSize)
	if _, err := io.ReadFull(r, header.Bytes()); err != nil {
		header.Release()
		return nil, err
	}

	data := header.Bytes()
	if binary.BigEndian.Uint16(data[0:2]) != Magic {
		header.Release()
		return nil, ErrMagic
	}

	frameLen := binary.BigEndian.Uint32(data[4:8])
	if frameLen < HeaderSize || frameLen > c.maxFrame {
		header.Release()
		return nil, ErrFrameLength
	}

	bodyLen := int(frameLen) - HeaderSize
	frame := buffer.MallocBytes(int(frameLen))
	copy(frame.Bytes()[:HeaderSize], data)
	header.Release()

	if bodyLen > 0 {
		if _, err := io.ReadFull(r, frame.Bytes()[HeaderSize:]); err != nil {
			frame.Release()
			return nil, err
		}
	}

	message := &Message{
		Type:  Type(binary.BigEndian.Uint16(frame.Bytes()[2:4])),
		Route: binary.BigEndian.Uint32(frame.Bytes()[8:12]),
		Seq:   binary.BigEndian.Uint64(frame.Bytes()[12:20]),
		Body:  frame.Bytes()[HeaderSize:],
		buf:   frame,
	}
	if err := c.Validate(message); err != nil {
		message.Release()
		return nil, err
	}
	return message, nil
}

// Encode 将 Message 编码到池化内存中，调用方必须 Release 返回值。
func (c *Codec) Encode(message *Message) (*buffer.Bytes, error) {
	if err := c.Validate(message); err != nil {
		return nil, err
	}
	if uint64(len(message.Body))+HeaderSize > uint64(c.maxFrame) {
		return nil, ErrFrameLength
	}

	frameLen := HeaderSize + len(message.Body)
	frame := buffer.MallocBytes(frameLen)
	data := frame.Bytes()
	binary.BigEndian.PutUint16(data[0:2], Magic)
	binary.BigEndian.PutUint16(data[2:4], uint16(message.Type))
	binary.BigEndian.PutUint32(data[4:8], uint32(frameLen))
	binary.BigEndian.PutUint32(data[8:12], message.Route)
	binary.BigEndian.PutUint64(data[12:20], message.Seq)
	copy(data[HeaderSize:], message.Body)
	return frame, nil
}

// Validate 校验包类型及字段组合。
func (c *Codec) Validate(message *Message) error {
	if message == nil {
		return ErrType
	}

	switch message.Type {
	case Req:
		if message.Route == 0 {
			return ErrRoute
		}
	case Rsp:
		if message.Route == 0 {
			return ErrRoute
		}
		if message.Seq == 0 {
			return ErrSeq
		}
	case Push:
		if message.Route == 0 {
			return ErrRoute
		}
		if message.Seq != 0 {
			return ErrSeq
		}
	case Ping, Pong:
		if message.Route != 0 {
			return ErrRoute
		}
		if message.Seq != 0 {
			return ErrSeq
		}
		if len(message.Body) != PingBodySize {
			return ErrBody
		}
	case BindReq, BindRsp:
		if message.Route != 0 {
			return ErrRoute
		}
		if message.Seq != 0 {
			return ErrSeq
		}
		if len(message.Body) > int(c.maxBind) {
			return ErrBody
		}
	default:
		return fmt.Errorf("%w: %d", ErrType, message.Type)
	}
	return nil
}

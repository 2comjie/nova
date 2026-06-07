package packet

import (
	"io"

	"github.com/2comjie/wali/core/buffer"
)

var globalPacker Packer

func init() {
	globalPacker = NewPacker()
}

type Packer interface {
	ReadBuffer(reader io.Reader) (buffer.Buffer, error)
	PackBuffer(messageType MessageType, route int32, seq int32, data buffer.Buffer) (buffer.Buffer, error)
	ToMessage(buff buffer.Buffer) Message
}

type defaultPacker struct {
	options *options
}

func NewPacker(options ...Option) Packer {
	op := defaultOptions()
	for _, option := range options {
		option(op)
	}
	p := &defaultPacker{options: op}
	return p
}

func (p *defaultPacker) ReadBuffer(reader io.Reader) (buffer.Buffer, error) {
	// 读取包大小
	sizeBuff := buffer.MallocBytes(4)
	defer sizeBuff.Release()
	_, err := io.ReadFull(reader, sizeBuff.Bytes())
	if err != nil {
		return nil, err
	}
	size := p.options.byteOrder.Uint32(sizeBuff.Bytes())
	if size == 0 || size > p.options.maxPacketSize {
		// 直接拒绝掉
		return nil, nil
	}

	// 读取消息体
	dataBuff := buffer.MallocBytes(int(size))
	_, err = io.ReadFull(reader, dataBuff.Bytes())
	if err != nil {
		dataBuff.Release() // 释放了
		return nil, err
	}
	return dataBuff, nil
}
func (p *defaultPacker) PackBuffer(messageType MessageType, route int32, seq int32, data buffer.Buffer) (buffer.Buffer, error) {
	dataLen := 0
	if data != nil {
		dataLen = data.Len()
	}

	// size = head(1) + route(4) + seq(4) + data
	bodySize := 9 + dataLen

	// 构造 header 各段（走内存池，不 alloc）
	sizeBuf := buffer.MallocBytes(4)
	p.options.byteOrder.PutUint32(sizeBuf.Bytes(), uint32(bodySize))

	headBuf := buffer.MallocBytes(1)
	headBuf.Bytes()[0] = byte(messageType)

	routeBuf := buffer.MallocBytes(4)
	p.options.byteOrder.PutUint32(routeBuf.Bytes(), uint32(route))

	seqBuf := buffer.MallocBytes(4)
	p.options.byteOrder.PutUint32(seqBuf.Bytes(), uint32(seq))

	// 零拷贝拼接: [size(4)][head(1)][route(4)][seq(4)][data]
	buf := buffer.NewNocopyBuffer(sizeBuf, headBuf, routeBuf, seqBuf)
	if data != nil {
		buf.Mount(data)
	}
	return buf, nil
}

func (p *defaultPacker) ToMessage(buff buffer.Buffer) Message {
	return Message{
		buff: buff,
	}
}

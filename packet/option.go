package packet

import (
	"encoding/binary"

	"github.com/2comjie/wali/core/bytes"
)

type options struct {
	byteOrder     binary.ByteOrder
	maxPacketSize uint32
}
type Option func(options *options)

func WithByteOrder(byteOrder binary.ByteOrder) Option {
	return func(options *options) {
		options.byteOrder = byteOrder
	}
}

func WithMaxPacketSize(maxPacketSize uint32) Option {
	return func(options *options) {
		options.maxPacketSize = maxPacketSize
	}
}

func defaultOptions() *options {
	return &options{
		byteOrder:     binary.BigEndian,
		maxPacketSize: 2 * bytes.MB,
	}
}

package diff

import (
	"bytes"
	"encoding/binary"
	"math"
)

type ValueReader struct {
	data   []byte
	offset int
	err    error
}

func NewValueReader(data []byte) *ValueReader {
	return &ValueReader{data: data}
}

func (r *ValueReader) Bool() bool {
	return r.readUvarint() != 0
}

func (r *ValueReader) Enum() int32 {
	return int32(r.readUvarint())
}

func (r *ValueReader) Int32() int32 {
	return int32(r.readUvarint())
}

func (r *ValueReader) Int64() int64 {
	return int64(r.readUvarint())
}

func (r *ValueReader) Sint32() int32 {
	value := uint32(r.readUvarint())
	return int32(value>>1) ^ -int32(value&1)
}

func (r *ValueReader) Sint64() int64 {
	value := r.readUvarint()
	return int64(value>>1) ^ -int64(value&1)
}

func (r *ValueReader) Uint32() uint32 {
	return uint32(r.readUvarint())
}

func (r *ValueReader) Uint64() uint64 {
	return r.readUvarint()
}

func (r *ValueReader) Fixed32() uint32 {
	return binary.LittleEndian.Uint32(r.read(4))
}

func (r *ValueReader) Sfixed32() int32 {
	return int32(r.Fixed32())
}

func (r *ValueReader) Float32() float32 {
	return math.Float32frombits(r.Fixed32())
}

func (r *ValueReader) Fixed64() uint64 {
	return binary.LittleEndian.Uint64(r.read(8))
}

func (r *ValueReader) Sfixed64() int64 {
	return int64(r.Fixed64())
}

func (r *ValueReader) Float64() float64 {
	return math.Float64frombits(r.Fixed64())
}

func (r *ValueReader) String() string {
	return string(r.Bytes())
}

func (r *ValueReader) Bytes() []byte {
	length := r.readUvarint()
	if r.err != nil {
		return nil
	}
	if length > uint64(len(r.data)-r.offset) {
		r.err = ErrInvalidData
		r.offset = len(r.data)
		return nil
	}
	value := r.data[r.offset : r.offset+int(length)]
	r.offset += int(length)
	return value
}

func (r *ValueReader) Remaining() []byte {
	if r.err != nil {
		return nil
	}
	value := r.data[r.offset:]
	r.offset = len(r.data)
	return value
}

func (r *ValueReader) Done() bool {
	return r.err != nil || r.offset == len(r.data)
}

func (r *ValueReader) Err() error {
	return r.err
}

func (r *ValueReader) readUvarint() uint64 {
	if r.err != nil {
		return 0
	}
	value, size := binary.Uvarint(r.data[r.offset:])
	if size <= 0 {
		r.err = ErrInvalidData
		r.offset = len(r.data)
		return 0
	}
	r.offset += size
	return value
}

func (r *ValueReader) read(size int) []byte {
	if r.err != nil {
		return make([]byte, size)
	}
	if len(r.data)-r.offset < size {
		r.err = ErrInvalidData
		r.offset = len(r.data)
		return make([]byte, size)
	}
	value := r.data[r.offset : r.offset+size]
	r.offset += size
	return value
}

func DecodeBool(data []byte) bool {
	value, _ := binary.Uvarint(data)
	return value != 0
}

func DecodeEnum(data []byte) int32 {
	return DecodeInt32(data)
}

func DecodeInt32(data []byte) int32 {
	value, _ := binary.Uvarint(data)
	return int32(value)
}

func DecodeInt64(data []byte) int64 {
	value, _ := binary.Uvarint(data)
	return int64(value)
}

func DecodeSint32(data []byte) int32 {
	value, _ := binary.Uvarint(data)
	return int32(uint32(value)>>1) ^ -int32(uint32(value)&1)
}

func DecodeSint64(data []byte) int64 {
	value, _ := binary.Uvarint(data)
	return int64(value>>1) ^ -int64(value&1)
}

func DecodeUint32(data []byte) uint32 {
	value, _ := binary.Uvarint(data)
	return uint32(value)
}

func DecodeUint64(data []byte) uint64 {
	value, _ := binary.Uvarint(data)
	return value
}

func DecodeFixed32(data []byte) uint32 {
	return binary.LittleEndian.Uint32(data)
}

func DecodeSfixed32(data []byte) int32 {
	return int32(DecodeFixed32(data))
}

func DecodeFloat32(data []byte) float32 {
	return math.Float32frombits(DecodeFixed32(data))
}

func DecodeFixed64(data []byte) uint64 {
	return binary.LittleEndian.Uint64(data)
}

func DecodeSfixed64(data []byte) int64 {
	return int64(DecodeFixed64(data))
}

func DecodeFloat64(data []byte) float64 {
	return math.Float64frombits(DecodeFixed64(data))
}

func DecodeString(data []byte) string {
	return string(data)
}

func DecodeBytes(data []byte) []byte {
	return bytes.Clone(data)
}

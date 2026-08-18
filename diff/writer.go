package diff

import (
	"encoding/binary"
	"math"
)

// 增量diff编码
// [Tag] [Value]
// int64 具体的值
// [Tag] --> OpType|FieldIndex 低5位|高位

type Writer struct {
	data []byte
}

func NewWriter(data []byte) *Writer {
	return &Writer{
		data: data,
	}
}
func (w *Writer) Data() []byte {
	return w.data
}

func (w *Writer) writeTag(fieldNumber uint32, operation Operation) {
	// 写入脏标记的信息到一个 64bit 里
	// operation最大是16 所以需要5位
	// 低5位是operation 高位是字段index
	tag := uint64(fieldNumber)<<5 | uint64(operation)
	w.data = binary.AppendUvarint(w.data, tag) // 编码到data里面
}

func (w *Writer) writeBlock(fieldNumber uint32, operation Operation, write func(*Writer)) {
	w.writeTag(fieldNumber, operation)
	lengthIndex := len(w.data)
	w.data = append(w.data, 0)
	write(w)

	size := len(w.data) - lengthIndex - 1
	if size < 128 {
		w.data[lengthIndex] = byte(size)
		return
	}

	var lengthData [10]byte
	length := binary.PutUvarint(lengthData[:], uint64(size))
	w.data = append(w.data, make([]byte, length-1)...)
	copy(w.data[lengthIndex+length:], w.data[lengthIndex+1:len(w.data)-length+1])
	copy(w.data[lengthIndex:lengthIndex+length], lengthData[:length])
}

func (w *Writer) Bool(fieldNumber uint32, value bool) {
	w.writeTag(fieldNumber, OperationSetVarint)
	w.AppendBool(value)
}

func (w *Writer) AppendBool(value bool) {
	if value {
		w.data = append(w.data, 1)
		return
	}
	w.data = append(w.data, 0)
}

func (w *Writer) Enum(fieldNumber uint32, value int32) {
	w.Int32(fieldNumber, value)
}

func (w *Writer) AppendEnum(value int32) {
	w.AppendInt32(value)
}

func (w *Writer) Int32(fieldNumber uint32, value int32) {
	w.writeTag(fieldNumber, OperationSetVarint)
	w.AppendInt32(value)
}

func (w *Writer) AppendInt32(value int32) {
	w.data = binary.AppendUvarint(w.data, uint64(int64(value)))
}

func (w *Writer) Int64(fieldNumber uint32, value int64) {
	w.writeTag(fieldNumber, OperationSetVarint)
	w.AppendInt64(value)
}

func (w *Writer) AppendInt64(value int64) {
	w.data = binary.AppendUvarint(w.data, uint64(value))
}

func (w *Writer) Sint32(fieldNumber uint32, value int32) {
	w.writeTag(fieldNumber, OperationSetVarint)
	w.AppendSint32(value)
}

func (w *Writer) AppendSint32(value int32) {
	w.data = binary.AppendUvarint(w.data, uint64(uint32(value<<1)^uint32(value>>31)))
}

func (w *Writer) Sint64(fieldNumber uint32, value int64) {
	w.writeTag(fieldNumber, OperationSetVarint)
	w.AppendSint64(value)
}

func (w *Writer) AppendSint64(value int64) {
	w.data = binary.AppendUvarint(w.data, uint64(value<<1)^uint64(value>>63))
}

func (w *Writer) Uint32(fieldNumber uint32, value uint32) {
	w.writeTag(fieldNumber, OperationSetVarint)
	w.AppendUint32(value)
}

func (w *Writer) AppendUint32(value uint32) {
	w.data = binary.AppendUvarint(w.data, uint64(value))
}

func (w *Writer) Uint64(fieldNumber uint32, value uint64) {
	w.writeTag(fieldNumber, OperationSetVarint)
	w.AppendUint64(value)
}

func (w *Writer) AppendUint64(value uint64) {
	w.data = binary.AppendUvarint(w.data, value)
}

func (w *Writer) Fixed32(fieldNumber uint32, value uint32) {
	w.writeTag(fieldNumber, OperationSetFixed32)
	w.AppendFixed32(value)
}

func (w *Writer) AppendFixed32(value uint32) {
	w.data = binary.LittleEndian.AppendUint32(w.data, value)
}

func (w *Writer) Sfixed32(fieldNumber uint32, value int32) {
	w.Fixed32(fieldNumber, uint32(value))
}

func (w *Writer) AppendSfixed32(value int32) {
	w.AppendFixed32(uint32(value))
}

func (w *Writer) Float32(fieldNumber uint32, value float32) {
	w.Fixed32(fieldNumber, math.Float32bits(value))
}

func (w *Writer) AppendFloat32(value float32) {
	w.AppendFixed32(math.Float32bits(value))
}

func (w *Writer) Fixed64(fieldNumber uint32, value uint64) {
	w.writeTag(fieldNumber, OperationSetFixed64)
	w.AppendFixed64(value)
}

func (w *Writer) AppendFixed64(value uint64) {
	w.data = binary.LittleEndian.AppendUint64(w.data, value)
}

func (w *Writer) Sfixed64(fieldNumber uint32, value int64) {
	w.Fixed64(fieldNumber, uint64(value))
}

func (w *Writer) AppendSfixed64(value int64) {
	w.AppendFixed64(uint64(value))
}

func (w *Writer) Float64(fieldNumber uint32, value float64) {
	w.Fixed64(fieldNumber, math.Float64bits(value))
}

func (w *Writer) AppendFloat64(value float64) {
	w.AppendFixed64(math.Float64bits(value))
}

func (w *Writer) String(fieldNumber uint32, value string) {
	w.writeTag(fieldNumber, OperationSetBytes)
	w.AppendString(value)
}

func (w *Writer) AppendString(value string) {
	w.data = binary.AppendUvarint(w.data, uint64(len(value)))
	w.data = append(w.data, value...)
}

func (w *Writer) Bytes(fieldNumber uint32, value []byte) {
	w.writeTag(fieldNumber, OperationSetBytes)
	w.AppendBytes(value)
}

func (w *Writer) AppendBytes(value []byte) {
	w.data = binary.AppendUvarint(w.data, uint64(len(value)))
	w.data = append(w.data, value...)
}

func (w *Writer) AppendRaw(value []byte) {
	w.data = append(w.data, value...)
}

func (w *Writer) Clear(fieldNumber uint32) {
	w.writeTag(fieldNumber, OperationClear)
}

func (w *Writer) Patch(fieldNumber uint32, writePatch func(*Writer)) {
	w.writeBlock(fieldNumber, OperationPatch, writePatch)
}

func (w *Writer) Replace(fieldNumber uint32, writeValue func(*Writer)) {
	w.writeBlock(fieldNumber, OperationReplace, writeValue)
}

func (w *Writer) ListAppend(fieldNumber uint32, writeValue func(*Writer)) {
	w.writeBlock(fieldNumber, OperationListAppend, writeValue)
}

func (w *Writer) ListInsert(fieldNumber uint32, index uint32, writeValue func(*Writer)) {
	w.writeBlock(fieldNumber, OperationListInsert, func(writer *Writer) {
		writer.AppendUint32(index)
		writeValue(writer)
	})
}

func (w *Writer) ListSet(fieldNumber uint32, index uint32, writeValue func(*Writer)) {
	w.writeBlock(fieldNumber, OperationListSet, func(writer *Writer) {
		writer.AppendUint32(index)
		writeValue(writer)
	})
}

func (w *Writer) ListDelete(fieldNumber uint32, index uint32) {
	w.writeBlock(fieldNumber, OperationListDelete, func(writer *Writer) {
		writer.AppendUint32(index)
	})
}

func (w *Writer) ListMove(fieldNumber uint32, from uint32, to uint32) {
	w.writeBlock(fieldNumber, OperationListMove, func(writer *Writer) {
		writer.AppendUint32(from)
		writer.AppendUint32(to)
	})
}

func (w *Writer) ListPatch(fieldNumber uint32, index uint32, writePatch func(*Writer)) {
	w.writeBlock(fieldNumber, OperationListPatch, func(writer *Writer) {
		writer.AppendUint32(index)
		writePatch(writer)
	})
}

func (w *Writer) MapPut(fieldNumber uint32, writeKey func(*Writer), writeValue func(*Writer)) {
	w.writeBlock(fieldNumber, OperationMapPut, func(writer *Writer) {
		writeKey(writer)
		writeValue(writer)
	})
}

func (w *Writer) MapDelete(fieldNumber uint32, writeKey func(*Writer)) {
	w.writeBlock(fieldNumber, OperationMapDelete, writeKey)
}

func (w *Writer) MapPatch(fieldNumber uint32, writeKey func(*Writer), writePatch func(*Writer)) {
	w.writeBlock(fieldNumber, OperationMapPatch, func(writer *Writer) {
		writeKey(writer)
		writePatch(writer)
	})
}

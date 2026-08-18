package diff

import (
	"encoding/binary"
	"errors"
	"math"
)

var ErrInvalidData = errors.New("diff: invalid data")

type Reader struct {
	data   []byte
	offset int
}

func NewReader(data []byte) *Reader {
	return &Reader{data: data}
}

func (r *Reader) Next() (fieldNumber uint32, operation Operation, payload []byte, ok bool, err error) {
	if r.offset == len(r.data) {
		return 0, 0, nil, false, nil
	}

	tag, size := binary.Uvarint(r.data[r.offset:])
	if size <= 0 || tag>>5 == 0 || tag>>5 > math.MaxUint32 {
		return 0, 0, nil, false, ErrInvalidData
	}
	r.offset += size

	fieldNumber = uint32(tag >> 5)
	operation = Operation(tag & 31)

	switch operation {
	case OperationSetVarint:
		_, size = binary.Uvarint(r.data[r.offset:])
		if size <= 0 {
			return 0, 0, nil, false, ErrInvalidData
		}
		payload = r.data[r.offset : r.offset+size]
		r.offset += size
	case OperationSetFixed32:
		if len(r.data)-r.offset < 4 {
			return 0, 0, nil, false, ErrInvalidData
		}
		payload = r.data[r.offset : r.offset+4]
		r.offset += 4
	case OperationSetFixed64:
		if len(r.data)-r.offset < 8 {
			return 0, 0, nil, false, ErrInvalidData
		}
		payload = r.data[r.offset : r.offset+8]
		r.offset += 8
	case OperationSetBytes,
		OperationPatch,
		OperationReplace,
		OperationListAppend,
		OperationListInsert,
		OperationListSet,
		OperationListDelete,
		OperationListMove,
		OperationListPatch,
		OperationMapPut,
		OperationMapDelete,
		OperationMapPatch:
		length, lengthSize := binary.Uvarint(r.data[r.offset:])
		if lengthSize <= 0 {
			return 0, 0, nil, false, ErrInvalidData
		}
		r.offset += lengthSize
		if length > uint64(len(r.data)-r.offset) {
			return 0, 0, nil, false, ErrInvalidData
		}
		payload = r.data[r.offset : r.offset+int(length)]
		r.offset += int(length)
	case OperationClear:
	default:
		return 0, 0, nil, false, ErrInvalidData
	}

	return fieldNumber, operation, payload, true, nil
}

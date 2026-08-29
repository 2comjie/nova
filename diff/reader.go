package diff

import (
	"encoding/binary"
	"errors"
	"math"
)

var ErrInvalidData = errors.New("diff: 无效的编码数据")

type EncodedPathNode struct {
	KeyType    PathKeyType
	FieldIndex uint32
	MapKey     []byte
}

type EncodedPatch struct {
	Path      []EncodedPathNode
	Operation Operation
	Value     []byte
}

type EncodedField struct {
	FieldIndex uint32
	Value      []byte
}

func DecodePatches(data []byte) ([]EncodedPatch, error) {
	patchCount, offset, ok := readUvarint(data, 0)
	if !ok || patchCount > uint64(len(data)) {
		return nil, ErrInvalidData
	}

	patches := make([]EncodedPatch, 0, int(patchCount))
	for range patchCount {
		pathData, nextOffset, ok := readBlock(data, offset)
		if !ok {
			return nil, ErrInvalidData
		}
		offset = nextOffset

		path, ok := decodePath(pathData)
		if !ok || len(path) == 0 || offset >= len(data) {
			return nil, ErrInvalidData
		}

		operation := Operation(data[offset])
		offset++
		if operation < PrimitiveSet || operation > SliceReplace {
			return nil, ErrInvalidData
		}

		value, nextOffset, ok := readBlock(data, offset)
		if !ok {
			return nil, ErrInvalidData
		}
		offset = nextOffset

		patches = append(patches, EncodedPatch{
			Path:      path,
			Operation: operation,
			Value:     value,
		})
	}

	if offset != len(data) {
		return nil, ErrInvalidData
	}
	return patches, nil
}

func DecodeFields(data []byte) ([]EncodedField, error) {
	fields := make([]EncodedField, 0, 8)
	for offset := 0; offset < len(data); {
		fieldIndex, nextOffset, ok := readUvarint(data, offset)
		if !ok || fieldIndex > math.MaxUint32 {
			return nil, ErrInvalidData
		}
		offset = nextOffset

		value, nextOffset, ok := readBlock(data, offset)
		if !ok {
			return nil, ErrInvalidData
		}
		offset = nextOffset

		fields = append(fields, EncodedField{
			FieldIndex: uint32(fieldIndex),
			Value:      value,
		})
	}
	return fields, nil
}

func DecodeValues(data []byte) ([][]byte, error) {
	values := make([][]byte, 0, 8)
	for offset := 0; offset < len(data); {
		value, nextOffset, ok := readBlock(data, offset)
		if !ok {
			return nil, ErrInvalidData
		}
		offset = nextOffset
		values = append(values, value)
	}
	return values, nil
}

func DecodePointerElement(data []byte) ([]byte, bool, error) {
	if len(data) == 0 || data[0] > 1 {
		return nil, false, ErrInvalidData
	}
	if data[0] == 0 {
		if len(data) != 1 {
			return nil, false, ErrInvalidData
		}
		return nil, false, nil
	}
	return data[1:], true, nil
}

func decodePath(data []byte) ([]EncodedPathNode, bool) {
	path := make([]EncodedPathNode, 0, 4)
	for offset := 0; offset < len(data); {
		tag, nextOffset, ok := readUvarint(data, offset)
		if !ok || tag>>2 > math.MaxUint32 {
			return nil, false
		}
		offset = nextOffset

		node := EncodedPathNode{
			KeyType:    PathKeyType(tag & 3),
			FieldIndex: uint32(tag >> 2),
		}
		switch node.KeyType {
		case PathField:
		case PathMap:
			key, nextOffset, ok := readBlock(data, offset)
			if !ok {
				return nil, false
			}
			node.MapKey = key
			offset = nextOffset
		default:
			return nil, false
		}
		path = append(path, node)
	}
	return path, true
}

func readBlock(data []byte, offset int) ([]byte, int, bool) {
	length, offset, ok := readUvarint(data, offset)
	if !ok || length > uint64(len(data)-offset) {
		return nil, 0, false
	}
	end := offset + int(length)
	return data[offset:end], end, true
}

func readUvarint(data []byte, offset int) (uint64, int, bool) {
	if offset >= len(data) {
		return 0, 0, false
	}
	value, size := binary.Uvarint(data[offset:])
	if size <= 0 {
		return 0, 0, false
	}
	return value, offset + size, true
}

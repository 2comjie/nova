package diff

import (
	"encoding/binary"
	"reflect"
)

type PathKeyType uint8

const (
	PathField PathKeyType = 0
	PathMap   PathKeyType = 1
)

type PathNode struct {
	KeyType    PathKeyType
	FieldIndex uint32
	MapKey     any
}

type Path []PathNode

type Operation uint8

const (
	PrimitiveSet Operation = 1

	PointerSet   Operation = 2
	PointerClear Operation = 3

	MapSet    Operation = 4
	MapDelete Operation = 5
	MapClear  Operation = 6

	SliceReplace Operation = 7
)

type Patch struct {
	Path      Path
	Operation Operation
	Value     any
}

type Writer struct {
	patches []Patch
}

func NewWriter() *Writer {
	return &Writer{}
}

func (w *Writer) WritePatch(patch Patch) {
	patch.Path = append(Path(nil), patch.Path...)

	switch patch.Operation {
	case PrimitiveSet, PointerSet, PointerClear, MapSet, MapDelete, MapClear, SliceReplace:
		w.mergeOverwrite(patch)
	default:
		panic("diff: 未知Patch操作")
	}
}

func (w *Writer) Len() int {
	return len(w.patches)
}

func (w *Writer) Range(fn func(Patch) bool) {
	for _, patch := range w.patches {
		if !fn(patch) {
			return
		}
	}
}

func (w *Writer) Reset() {
	clear(w.patches)
	w.patches = w.patches[:0]
}

func (w *Writer) mergeOverwrite(patch Patch) {
	writeIndex := 0
	for _, current := range w.patches {
		if pathWithin(patch.Path, current.Path) {
			continue
		}
		w.patches[writeIndex] = current
		writeIndex++
	}

	clear(w.patches[writeIndex:])
	w.patches = w.patches[:writeIndex]
	w.patches = append(w.patches, patch)
}

func pathWithin(parent Path, child Path) bool {
	if len(parent) > len(child) {
		return false
	}

	for index, parentNode := range parent {
		childNode := child[index]
		if parentNode.FieldIndex != childNode.FieldIndex {
			return false
		}

		if index == len(parent)-1 && parentNode.KeyType == PathField {
			return true
		}
		if !samePathNode(parentNode, childNode) {
			return false
		}
	}
	return true
}

func samePathNode(left PathNode, right PathNode) bool {
	if left.FieldIndex != right.FieldIndex || left.KeyType != right.KeyType {
		return false
	}
	if left.KeyType == PathMap {
		return left.MapKey == right.MapKey
	}
	return true
}

func (w *Writer) Commit() []byte {
	data := binary.AppendUvarint(nil, uint64(len(w.patches)))
	for _, patch := range w.patches {
		var lengthIndex int

		data, lengthIndex = beginValue(data)
		data = appendPath(data, patch.Path)
		data = endValue(data, lengthIndex)

		data = append(data, byte(patch.Operation))

		data, lengthIndex = beginValue(data)
		data = appendPatchValue(data, patch.Value)
		data = endValue(data, lengthIndex)
	}
	return data
}

func appendPath(data []byte, path Path) []byte {
	for _, node := range path {
		tag := uint64(node.FieldIndex)<<2 | uint64(node.KeyType)
		data = binary.AppendUvarint(data, tag)
		if node.KeyType != PathMap {
			continue
		}

		var lengthIndex int
		data, lengthIndex = beginValue(data)
		data = appendPrimitive(data, node.MapKey)
		data = endValue(data, lengthIndex)
	}
	return data
}

func appendPatchValue(data []byte, value any) []byte {
	if value == nil {
		return data
	}

	reflectValue := reflect.ValueOf(value)
	if reflectValue.Kind() == reflect.Pointer && reflectValue.IsNil() {
		return data
	}

	if diffValue, ok := value.(interface {
		AppendDiffValue([]byte) []byte
	}); ok {
		return diffValue.AppendDiffValue(data)
	}
	return appendPrimitive(data, value)
}

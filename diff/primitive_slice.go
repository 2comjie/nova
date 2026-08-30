package diff

import (
	"github.com/2comjie/nova/generic"
)

type PrimitiveSlice[T generic.Primitive] struct {
	_ noCopy

	values []T

	parent    *Object
	diffIndex uint32
}

func (s *PrimitiveSlice[T]) Init(parent *Object, diffIndex uint32) {
	s.parent = parent
	s.diffIndex = diffIndex
}

func (s *PrimitiveSlice[T]) Initialized() bool {
	return s.parent != nil
}

func (s *PrimitiveSlice[T]) Len() int {
	return len(s.values)
}

func (s *PrimitiveSlice[T]) GetValue(index int) T {
	return s.values[index]
}

func (s *PrimitiveSlice[T]) SetValue(index int, value T) bool {
	oldValue := s.values[index]
	if oldValue == value {
		return false
	}

	value, event, accepted := beforeSliceChange(s.parent, s.diffIndex, ChangeSliceSet, index, index, oldValue, true, value, true)
	if !accepted || oldValue == value {
		return false
	}

	s.values[index] = value
	s.writePatch()
	afterSliceChange(s.parent, s.diffIndex, event)
	return true
}

func (s *PrimitiveSlice[T]) Append(value T) {
	var zero T
	value, event, accepted := beforeSliceChange(s.parent, s.diffIndex, ChangeSliceAppend, len(s.values), len(s.values), zero, false, value, true)
	if !accepted {
		return
	}

	s.values = append(s.values, value)
	s.writePatch()
	afterSliceChange(s.parent, s.diffIndex, event)
}

func (s *PrimitiveSlice[T]) Insert(index int, value T) {
	var zero T
	value, event, accepted := beforeSliceChange(s.parent, s.diffIndex, ChangeSliceInsert, index, index, zero, false, value, true)
	if !accepted {
		return
	}

	s.values = append(s.values, zero)
	copy(s.values[index+1:], s.values[index:])
	s.values[index] = value
	s.writePatch()
	afterSliceChange(s.parent, s.diffIndex, event)
}

func (s *PrimitiveSlice[T]) Delete(index int) T {
	value := s.values[index]
	var zero T
	_, event, accepted := beforeSliceChange(s.parent, s.diffIndex, ChangeSliceDelete, index, index, value, true, zero, false)
	if !accepted {
		return zero
	}

	lastIndex := len(s.values) - 1
	copy(s.values[index:], s.values[index+1:])

	s.values[lastIndex] = zero
	s.values = s.values[:lastIndex]

	s.writePatch()
	afterSliceChange(s.parent, s.diffIndex, event)
	return value
}

func (s *PrimitiveSlice[T]) Move(index int, toIndex int) bool {
	if index == toIndex {
		return false
	}

	value := s.values[index]
	value, event, accepted := beforeSliceChange(s.parent, s.diffIndex, ChangeSliceMove, index, toIndex, value, true, value, true)
	if !accepted {
		return false
	}

	if index < toIndex {
		copy(s.values[index:toIndex], s.values[index+1:toIndex+1])
	} else {
		copy(s.values[toIndex+1:index+1], s.values[toIndex:index])
	}
	s.values[toIndex] = value

	s.writePatch()
	afterSliceChange(s.parent, s.diffIndex, event)
	return true
}

func (s *PrimitiveSlice[T]) Clear() bool {
	if len(s.values) == 0 {
		return false
	}

	var zero T
	_, event, accepted := beforeSliceChange(s.parent, s.diffIndex, ChangeSliceClear, 0, 0, zero, false, zero, false)
	if !accepted {
		return false
	}

	s.values = nil
	s.writePatch()
	afterSliceChange(s.parent, s.diffIndex, event)
	return true
}

func (s *PrimitiveSlice[T]) Range(fn func(index int, value T) bool) {
	for index, value := range s.values {
		if !fn(index, value) {
			return
		}
	}
}

func (s *PrimitiveSlice[T]) AppendValue(data []byte, diffIndex uint32) []byte {
	if len(s.values) == 0 {
		return data
	}

	data, fieldLengthIndex := beginField(data, diffIndex)
	data = s.AppendDiffValue(data)
	return endValue(data, fieldLengthIndex)
}

func (s *PrimitiveSlice[T]) AppendDiffValue(data []byte) []byte {
	for _, value := range s.values {
		var lengthIndex int
		data, lengthIndex = beginValue(data)
		data = appendPrimitive(data, value)
		data = endValue(data, lengthIndex)
	}
	return data
}

func (s *PrimitiveSlice[T]) writePatch() {
	s.parent.writeChildPatch(s.diffIndex, nil, SliceReplace, s)
}

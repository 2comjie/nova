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

func (s *PrimitiveSlice[T]) Len() int {
	return len(s.values)
}

// LoadSnapshot 接管基线数据，不记录增量。调用方不再修改传入的 slice。
func (s *PrimitiveSlice[T]) LoadSnapshot(values []T) {
	s.values = values
}

func (s *PrimitiveSlice[T]) GetValue(index int) T {
	return s.values[index]
}

func (s *PrimitiveSlice[T]) SetValue(index int, value T) bool {
	oldValue := s.values[index]
	if oldValue == value {
		return false
	}

	s.values[index] = value
	s.writePatch()
	return true
}

func (s *PrimitiveSlice[T]) Append(value T) {
	s.values = append(s.values, value)
	s.writePatch()
}

func (s *PrimitiveSlice[T]) Insert(index int, value T) {
	var zero T
	s.values = append(s.values, zero)
	copy(s.values[index+1:], s.values[index:])
	s.values[index] = value
	s.writePatch()
}

func (s *PrimitiveSlice[T]) Delete(index int) T {
	value := s.values[index]
	var zero T
	lastIndex := len(s.values) - 1
	copy(s.values[index:], s.values[index+1:])

	s.values[lastIndex] = zero
	s.values = s.values[:lastIndex]

	s.writePatch()
	return value
}

func (s *PrimitiveSlice[T]) Move(index int, toIndex int) bool {
	if index == toIndex {
		return false
	}

	value := s.values[index]

	if index < toIndex {
		copy(s.values[index:toIndex], s.values[index+1:toIndex+1])
	} else {
		copy(s.values[toIndex+1:index+1], s.values[toIndex:index])
	}
	s.values[toIndex] = value

	s.writePatch()
	return true
}

func (s *PrimitiveSlice[T]) Clear() bool {
	if len(s.values) == 0 {
		return false
	}

	s.values = nil
	s.writePatch()
	return true
}

func (s *PrimitiveSlice[T]) Range(fn func(index int, value T) bool) {
	for index, value := range s.values {
		if !fn(index, value) {
			return
		}
	}
}

func (s *PrimitiveSlice[T]) writePatch() {
	s.parent.writeChildPatch(s.diffIndex, nil, SliceReplace, s.values)
}

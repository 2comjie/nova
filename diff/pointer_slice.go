package diff

type PointerSlice[V PointerValue] struct {
	_ noCopy

	values []V

	parent    *Object
	diffIndex uint32
}

func (s *PointerSlice[V]) Init(parent *Object, diffIndex uint32) {
	s.parent = parent
	s.diffIndex = diffIndex

	var zero V
	for index, value := range s.values {
		if value != zero {
			value.AddParent(s, index)
		}
	}
}

func (s *PointerSlice[V]) Len() int {
	return len(s.values)
}

func (s *PointerSlice[V]) GetValue(index int) V {
	return s.values[index]
}

func (s *PointerSlice[V]) SetValue(index int, value V) bool {
	oldValue := s.values[index]
	if oldValue == value {
		return false
	}

	var zero V
	if oldValue != zero {
		oldValue.RemoveParent(s, index)
	}

	s.values[index] = value
	if value != zero {
		value.AddParent(s, index)
	}

	s.writePatch()
	return true
}

func (s *PointerSlice[V]) Append(value V) {
	index := len(s.values)
	s.values = append(s.values, value)

	var zero V
	if value != zero {
		value.AddParent(s, index)
	}
	s.writePatch()
}

func (s *PointerSlice[V]) Insert(index int, value V) {
	var zero V
	for currentIndex := len(s.values) - 1; currentIndex >= index; currentIndex-- {
		currentValue := s.values[currentIndex]
		if currentValue != zero {
			currentValue.RemoveParent(s, currentIndex)
			currentValue.AddParent(s, currentIndex+1)
		}
	}

	s.values = append(s.values, zero)
	copy(s.values[index+1:], s.values[index:])
	s.values[index] = value

	if value != zero {
		value.AddParent(s, index)
	}
	s.writePatch()
}

func (s *PointerSlice[V]) Delete(index int) V {
	value := s.values[index]
	var zero V
	if value != zero {
		value.RemoveParent(s, index)
	}

	for currentIndex := index + 1; currentIndex < len(s.values); currentIndex++ {
		currentValue := s.values[currentIndex]
		if currentValue != zero {
			currentValue.RemoveParent(s, currentIndex)
			currentValue.AddParent(s, currentIndex-1)
		}
	}

	lastIndex := len(s.values) - 1
	copy(s.values[index:], s.values[index+1:])
	s.values[lastIndex] = zero
	s.values = s.values[:lastIndex]

	s.writePatch()
	return value
}

func (s *PointerSlice[V]) Move(index int, toIndex int) bool {
	if index == toIndex {
		return false
	}

	value := s.values[index]
	var zero V
	if value != zero {
		value.RemoveParent(s, index)
	}

	if index < toIndex {
		for currentIndex := index + 1; currentIndex <= toIndex; currentIndex++ {
			currentValue := s.values[currentIndex]
			if currentValue != zero {
				currentValue.RemoveParent(s, currentIndex)
				currentValue.AddParent(s, currentIndex-1)
			}
		}
		copy(s.values[index:toIndex], s.values[index+1:toIndex+1])
	} else {
		for currentIndex := toIndex; currentIndex < index; currentIndex++ {
			currentValue := s.values[currentIndex]
			if currentValue != zero {
				currentValue.RemoveParent(s, currentIndex)
				currentValue.AddParent(s, currentIndex+1)
			}
		}
		copy(s.values[toIndex+1:index+1], s.values[toIndex:index])
	}

	s.values[toIndex] = value
	if value != zero {
		value.AddParent(s, toIndex)
	}

	s.writePatch()
	return true
}

func (s *PointerSlice[V]) Clear() bool {
	if len(s.values) == 0 {
		return false
	}

	var zero V
	for index, value := range s.values {
		if value != zero {
			value.RemoveParent(s, index)
		}
	}
	s.values = nil
	s.writePatch()
	return true
}

func (s *PointerSlice[V]) Range(fn func(index int, value V) bool) {
	for index, value := range s.values {
		if !fn(index, value) {
			return
		}
	}
}

func (s *PointerSlice[V]) AppendValue(data []byte, diffIndex uint32) []byte {
	if len(s.values) == 0 {
		return data
	}

	data, fieldLengthIndex := beginField(data, diffIndex)
	data = s.AppendDiffValue(data)
	return endValue(data, fieldLengthIndex)
}

func (s *PointerSlice[V]) AppendDiffValue(data []byte) []byte {
	var zero V
	for _, value := range s.values {
		var lengthIndex int
		data, lengthIndex = beginValue(data)
		if value == zero {
			data = append(data, 0)
		} else {
			data = append(data, 1)
			data = value.AppendDiffValue(data)
		}
		data = endValue(data, lengthIndex)
	}
	return data
}

func (s *PointerSlice[V]) writeChildPatch(_ any, _ *pathNode, _ Operation, _ any) {
	s.writePatch()
}

func (s *PointerSlice[V]) writePatch() {
	s.parent.writeChildPatch(s.diffIndex, nil, SliceReplace, s)
}

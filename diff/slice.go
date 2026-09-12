package diff

type Slice[V any] struct {
	_ noCopy

	values []V

	parent    *Object
	diffIndex uint32
}

func (s *Slice[V]) Init(parent *Object, diffIndex uint32) {
	s.parent = parent
	s.diffIndex = diffIndex
}

func (s *Slice[V]) Len() int {
	return len(s.values)
}

func (s *Slice[V]) LoadSnapshot(values []V) {
	for index, value := range s.values {
		if object := objectValue(value); object != nil {
			object.RemoveParent(s, index)
		}
	}
	s.values = values
	for index, value := range values {
		if object := objectValue(value); object != nil {
			object.InitLink(nil)
			object.AddParent(s, index)
		}
	}
}

func (s *Slice[V]) GetValue(index int) V {
	return s.values[index]
}

func (s *Slice[V]) SetValue(index int, value V) bool {
	oldValue := s.values[index]
	if equalValue(oldValue, value) {
		return false
	}

	if object := objectValue(oldValue); object != nil {
		object.RemoveParent(s, index)
	}

	s.values[index] = value
	if object := objectValue(value); object != nil {
		object.InitLink(nil)
		object.AddParent(s, index)
	}

	s.writePatch()
	return true
}

func (s *Slice[V]) Append(value V) {
	index := len(s.values)
	s.values = append(s.values, value)

	if object := objectValue(value); object != nil {
		object.InitLink(nil)
		object.AddParent(s, index)
	}
	s.writePatch()
}

func (s *Slice[V]) Insert(index int, value V) {
	for currentIndex := len(s.values) - 1; currentIndex >= index; currentIndex-- {
		currentValue := s.values[currentIndex]
		if object := objectValue(currentValue); object != nil {
			object.RemoveParent(s, currentIndex)
			object.AddParent(s, currentIndex+1)
		}
	}

	var zero V
	s.values = append(s.values, zero)
	copy(s.values[index+1:], s.values[index:])
	s.values[index] = value

	if object := objectValue(value); object != nil {
		object.InitLink(nil)
		object.AddParent(s, index)
	}
	s.writePatch()
}

func (s *Slice[V]) Delete(index int) V {
	value := s.values[index]
	if object := objectValue(value); object != nil {
		object.RemoveParent(s, index)
	}

	for currentIndex := index + 1; currentIndex < len(s.values); currentIndex++ {
		currentValue := s.values[currentIndex]
		if object := objectValue(currentValue); object != nil {
			object.RemoveParent(s, currentIndex)
			object.AddParent(s, currentIndex-1)
		}
	}

	lastIndex := len(s.values) - 1
	copy(s.values[index:], s.values[index+1:])
	var zero V
	s.values[lastIndex] = zero
	s.values = s.values[:lastIndex]

	s.writePatch()
	return value
}

func (s *Slice[V]) Move(index int, toIndex int) bool {
	if index == toIndex {
		return false
	}

	oldValue := s.values[index]
	if object := objectValue(oldValue); object != nil {
		object.RemoveParent(s, index)
	}

	if index < toIndex {
		for currentIndex := index + 1; currentIndex <= toIndex; currentIndex++ {
			currentValue := s.values[currentIndex]
			if object := objectValue(currentValue); object != nil {
				object.RemoveParent(s, currentIndex)
				object.AddParent(s, currentIndex-1)
			}
		}
		copy(s.values[index:toIndex], s.values[index+1:toIndex+1])
	} else {
		for currentIndex := toIndex; currentIndex < index; currentIndex++ {
			currentValue := s.values[currentIndex]
			if object := objectValue(currentValue); object != nil {
				object.RemoveParent(s, currentIndex)
				object.AddParent(s, currentIndex+1)
			}
		}
		copy(s.values[toIndex+1:index+1], s.values[toIndex:index])
	}

	s.values[toIndex] = oldValue
	if object := objectValue(oldValue); object != nil {
		object.InitLink(nil)
		object.AddParent(s, toIndex)
	}

	s.writePatch()
	return true
}

func (s *Slice[V]) Clear() bool {
	if len(s.values) == 0 {
		return false
	}

	for index, value := range s.values {
		if object := objectValue(value); object != nil {
			object.RemoveParent(s, index)
		}
	}
	s.values = nil
	s.writePatch()
	return true
}

func (s *Slice[V]) Range(fn func(index int, value V) bool) {
	for index, value := range s.values {
		if !fn(index, value) {
			return
		}
	}
}

func (s *Slice[V]) writeChildPatch(_ any, _ Path, _ Operation, _ any) {
	s.writePatch()
}

func (s *Slice[V]) writePatch() {
	s.parent.writeChildPatch(s.diffIndex, nil, SliceReplace, s.values)
}

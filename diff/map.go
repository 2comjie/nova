package diff

type Map[K comparable, V any] struct {
	_ noCopy

	values    map[K]V
	parent    *Object
	diffIndex uint32
}

func (m *Map[K, V]) Init(parent *Object, diffIndex uint32) {
	m.parent = parent
	m.diffIndex = diffIndex
}

func (m *Map[K, V]) Len() int {
	return len(m.values)
}

func (m *Map[K, V]) LoadSnapshot(values map[K]V) {
	for key, value := range m.values {
		if object := objectValue(value); object != nil {
			object.RemoveParent(m, key)
		}
	}
	m.values = values
	for key, value := range values {
		if object := objectValue(value); object != nil {
			object.InitLink(nil)
			object.AddParent(m, key)
		} else if _, ok := any(value).(ObjectValue); ok {
			delete(values, key)
		}
	}
}

func (m *Map[K, V]) Load(key K) (V, bool) {
	value, exists := m.values[key]
	return value, exists
}

func (m *Map[K, V]) Store(key K, value V) bool {
	object := objectValue(value)
	if _, ok := any(value).(ObjectValue); ok && object == nil {
		return m.Delete(key)
	}
	oldValue, exists := m.values[key]
	if exists && equalValue(oldValue, value) {
		return false
	}
	if old := objectValue(oldValue); old != nil {
		old.RemoveParent(m, key)
	}
	if m.values == nil {
		m.values = make(map[K]V)
	}
	m.values[key] = value
	if object != nil {
		object.InitLink(nil)
		object.AddParent(m, key)
	}
	m.writeChildPatch(key, nil, MapSet, value)
	return true
}

func (m *Map[K, V]) Delete(key K) bool {
	value, exists := m.values[key]
	if !exists {
		return false
	}
	if object := objectValue(value); object != nil {
		object.RemoveParent(m, key)
	}
	delete(m.values, key)
	m.writeChildPatch(key, nil, MapDelete, nil)
	return true
}

func (m *Map[K, V]) Clear() bool {
	if len(m.values) == 0 {
		return false
	}
	for key, value := range m.values {
		if object := objectValue(value); object != nil {
			object.RemoveParent(m, key)
		}
	}
	m.values = nil
	m.parent.writeChildPatch(m.diffIndex, nil, MapClear, nil)
	return true
}

func (m *Map[K, V]) Range(fn func(K, V) bool) {
	for key, value := range m.values {
		if !fn(key, value) {
			return
		}
	}
}

func (m *Map[K, V]) writeChildPatch(key any, childPath Path, operation Operation, value any) {
	path := make(Path, len(childPath)+1)
	path[0] = PathNode{KeyType: PathMap, FieldIndex: m.diffIndex, MapKey: key}
	copy(path[1:], childPath)
	m.parent.writePatch(path, operation, value)
}

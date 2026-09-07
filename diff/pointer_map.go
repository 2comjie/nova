package diff

import "github.com/2comjie/nova/generic"

type PointerMap[K generic.Primitive, V PointerValue] struct {
	_ noCopy

	values map[K]V

	parent    *Object
	diffIndex uint32
}

func (m *PointerMap[K, V]) Init(parent *Object, diffIndex uint32) {
	m.parent = parent
	m.diffIndex = diffIndex
}

func (m *PointerMap[K, V]) Len() int {
	return len(m.values)
}

// LoadSnapshot 接管基线数据并更新父子链接，不记录增量。
// values 的元素必须非空，调用方不再修改传入的 map。
func (m *PointerMap[K, V]) LoadSnapshot(values map[K]V) {
	for key, value := range m.values {
		value.RemoveParent(m, key)
	}
	m.values = values
	for key, value := range values {
		value.InitLink(nil)
		value.AddParent(m, key)
	}
}

func (m *PointerMap[K, V]) Load(key K) (V, bool) {
	value, exists := m.values[key]
	return value, exists
}

func (m *PointerMap[K, V]) Store(key K, value V) bool {
	var zero V
	if value == zero {
		return m.Delete(key)
	}

	oldValue, exists := m.values[key]
	if exists && oldValue == value {
		return false
	}

	if exists {
		oldValue.RemoveParent(m, key)
	}

	value.InitLink(nil)
	if m.values == nil {
		m.values = make(map[K]V)
	}
	m.values[key] = value
	value.AddParent(m, key)

	m.writePatch(key, nil, MapSet, value)
	return true
}

func (m *PointerMap[K, V]) Delete(key K) bool {
	value, exists := m.values[key]
	if !exists {
		return false
	}

	value.RemoveParent(m, key)
	delete(m.values, key)
	m.writePatch(key, nil, MapDelete, nil)
	return true
}

func (m *PointerMap[K, V]) Clear() bool {
	if len(m.values) == 0 {
		return false
	}

	for key, value := range m.values {
		value.RemoveParent(m, key)
	}
	m.values = nil
	m.parent.writeChildPatch(m.diffIndex, nil, MapClear, nil)
	return true
}

func (m *PointerMap[K, V]) Range(fn func(K, V) bool) {
	for key, value := range m.values {
		if !fn(key, value) {
			return
		}
	}
}

func (m *PointerMap[K, V]) writeChildPatch(key any, childPath Path, operation Operation, value any) {
	m.writePatch(key.(K), childPath, operation, value)
}

func (m *PointerMap[K, V]) writePatch(key K, childPath Path, operation Operation, value any) {
	path := make(Path, len(childPath)+1)
	path[0] = PathNode{KeyType: PathMap, FieldIndex: m.diffIndex, MapKey: key}
	copy(path[1:], childPath)
	m.parent.writePatch(path, operation, value)
}

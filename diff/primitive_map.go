package diff

import "github.com/2comjie/nova/generic"

type PrimitiveMap[K generic.Primitive, V generic.Primitive] struct {
	_ noCopy

	values map[K]V

	parent    *Object
	diffIndex uint32
}

func (m *PrimitiveMap[K, V]) Init(parent *Object, diffIndex uint32) {
	m.parent = parent
	m.diffIndex = diffIndex
}

func (m *PrimitiveMap[K, V]) Len() int {
	return len(m.values)
}

// LoadSnapshot 接管基线数据，不记录增量。调用方不再修改传入的 map。
func (m *PrimitiveMap[K, V]) LoadSnapshot(values map[K]V) {
	m.values = values
}

func (m *PrimitiveMap[K, V]) Load(key K) (V, bool) {
	value, exists := m.values[key]
	return value, exists
}

func (m *PrimitiveMap[K, V]) Store(key K, value V) bool {
	oldValue, exists := m.values[key]
	if exists && oldValue == value {
		return false
	}

	if m.values == nil {
		m.values = make(map[K]V)
	}
	m.values[key] = value

	m.writePatch(key, MapSet, value)
	return true
}

func (m *PrimitiveMap[K, V]) Delete(key K) bool {
	_, exists := m.values[key]
	if !exists {
		return false
	}

	delete(m.values, key)
	m.writePatch(key, MapDelete, nil)
	return true
}

func (m *PrimitiveMap[K, V]) Clear() bool {
	if len(m.values) == 0 {
		return false
	}

	m.values = nil
	m.parent.writeChildPatch(m.diffIndex, nil, MapClear, nil)
	return true
}

func (m *PrimitiveMap[K, V]) Range(fn func(K, V) bool) {
	for key, value := range m.values {
		if !fn(key, value) {
			return
		}
	}
}

func (m *PrimitiveMap[K, V]) writePatch(key K, operation Operation, value any) {
	m.parent.writePatch(Path{{KeyType: PathMap, FieldIndex: m.diffIndex, MapKey: key}}, operation, value)
}

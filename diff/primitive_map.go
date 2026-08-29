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
	if _, exists := m.values[key]; !exists {
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

func (m *PrimitiveMap[K, V]) AppendValue(data []byte, diffIndex uint32) []byte {
	if len(m.values) == 0 {
		return data
	}

	data, fieldLengthIndex := beginField(data, diffIndex)
	for key, value := range m.values {
		var lengthIndex int
		data, lengthIndex = beginValue(data)
		data = appendPrimitive(data, key)
		data = endValue(data, lengthIndex)

		data, lengthIndex = beginValue(data)
		data = appendPrimitive(data, value)
		data = endValue(data, lengthIndex)
	}
	return endValue(data, fieldLengthIndex)
}

func (m *PrimitiveMap[K, V]) writePatch(key K, operation Operation, value any) {
	node := pathNode{
		fieldIndex: m.diffIndex,
		keyType:    PathMap,
		key:        key,
	}
	m.parent.writePatch(&node, operation, value)
}

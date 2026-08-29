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

	var zero V
	for key, value := range m.values {
		if value != zero {
			value.AddParent(m, key)
		}
	}
}

func (m *PointerMap[K, V]) Len() int {
	return len(m.values)
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

func (m *PointerMap[K, V]) AppendValue(data []byte, diffIndex uint32) []byte {
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
		data = value.AppendDiffValue(data)
		data = endValue(data, lengthIndex)
	}
	return endValue(data, fieldLengthIndex)
}

func (m *PointerMap[K, V]) writeChildPatch(key any, child *pathNode, operation Operation, value any) {
	m.writePatch(key.(K), child, operation, value)
}

func (m *PointerMap[K, V]) writePatch(key K, child *pathNode, operation Operation, value any) {
	node := pathNode{
		next:       child,
		fieldIndex: m.diffIndex,
		keyType:    PathMap,
		key:        key,
	}
	m.parent.writePatch(&node, operation, value)
}

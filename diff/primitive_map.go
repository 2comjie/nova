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

	value, event, accepted := beforeMapChange(m.parent, m.diffIndex, ChangeMapStore, key, true, oldValue, exists, value, true)
	if !accepted {
		return false
	}
	if event != nil && !event.newExists {
		if !exists {
			return false
		}
		delete(m.values, key)
		event.operation = ChangeMapDelete
		m.writePatch(key, MapDelete, nil)
		afterMapChange(m.parent, m.diffIndex, event)
		return true
	}
	if exists && oldValue == value {
		return false
	}

	if m.values == nil {
		m.values = make(map[K]V)
	}
	m.values[key] = value

	m.writePatch(key, MapSet, value)
	afterMapChange(m.parent, m.diffIndex, event)
	return true
}

func (m *PrimitiveMap[K, V]) Delete(key K) bool {
	value, exists := m.values[key]
	if !exists {
		return false
	}

	var zero V
	newValue, event, accepted := beforeMapChange(m.parent, m.diffIndex, ChangeMapDelete, key, true, value, true, zero, false)
	if !accepted {
		return false
	}
	if event != nil && event.newExists {
		if value == newValue {
			return false
		}
		m.values[key] = newValue
		event.operation = ChangeMapStore
		m.writePatch(key, MapSet, newValue)
		afterMapChange(m.parent, m.diffIndex, event)
		return true
	}

	delete(m.values, key)
	m.writePatch(key, MapDelete, nil)
	afterMapChange(m.parent, m.diffIndex, event)
	return true
}

func (m *PrimitiveMap[K, V]) Clear() bool {
	if len(m.values) == 0 {
		return false
	}

	var key K
	var value V
	_, event, accepted := beforeMapChange(m.parent, m.diffIndex, ChangeMapClear, key, false, value, false, value, false)
	if !accepted {
		return false
	}

	m.values = nil
	m.parent.writeChildPatch(m.diffIndex, nil, MapClear, nil)
	afterMapChange(m.parent, m.diffIndex, event)
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
	m.parent.writePatch(Path{{KeyType: PathMap, FieldIndex: m.diffIndex, MapKey: key}}, operation, value)
}

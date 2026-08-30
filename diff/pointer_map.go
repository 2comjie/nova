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

	value, event, accepted := beforeMapChange(m.parent, m.diffIndex, ChangeMapStore, key, true, oldValue, exists, value, true)
	if !accepted || exists && oldValue == value {
		return false
	}
	if event != nil && !event.newExists || value == zero {
		if !exists {
			return false
		}
		oldValue.RemoveParent(m, key)
		delete(m.values, key)
		event.operation = ChangeMapDelete
		event.newExists = false
		m.writePatch(key, nil, MapDelete, nil)
		afterMapChange(m.parent, m.diffIndex, event)
		return true
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
	afterMapChange(m.parent, m.diffIndex, event)
	return true
}

func (m *PointerMap[K, V]) Delete(key K) bool {
	value, exists := m.values[key]
	if !exists {
		return false
	}

	var zero V
	newValue, event, accepted := beforeMapChange(m.parent, m.diffIndex, ChangeMapDelete, key, true, value, true, zero, false)
	if !accepted {
		return false
	}
	if event != nil && event.newExists && newValue != zero {
		if value == newValue {
			return false
		}
		value.RemoveParent(m, key)
		m.values[key] = newValue
		newValue.AddParent(m, key)
		event.operation = ChangeMapStore
		m.writePatch(key, nil, MapSet, newValue)
		afterMapChange(m.parent, m.diffIndex, event)
		return true
	}

	value.RemoveParent(m, key)
	delete(m.values, key)
	m.writePatch(key, nil, MapDelete, nil)
	afterMapChange(m.parent, m.diffIndex, event)
	return true
}

func (m *PointerMap[K, V]) Clear() bool {
	if len(m.values) == 0 {
		return false
	}

	var key K
	var value V
	_, event, accepted := beforeMapChange(m.parent, m.diffIndex, ChangeMapClear, key, false, value, false, value, false)
	if !accepted {
		return false
	}

	for key, value := range m.values {
		value.RemoveParent(m, key)
	}
	m.values = nil
	m.parent.writeChildPatch(m.diffIndex, nil, MapClear, nil)
	afterMapChange(m.parent, m.diffIndex, event)
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

func (m *PointerMap[K, V]) writeChildPatch(key any, childPath Path, operation Operation, value any) {
	m.writePatch(key.(K), childPath, operation, value)
}

func (m *PointerMap[K, V]) dispatchChildChange(key any, childPath listenerPath, phase listenerPhase, event *changeEvent) {
	m.parent.dispatchChange(prependListenerPath(listenerPathNode{
		selector:   listenerMapKey,
		fieldIndex: m.diffIndex,
		key:        key,
	}, childPath), phase, event)
}

func (m *PointerMap[K, V]) writePatch(key K, childPath Path, operation Operation, value any) {
	path := make(Path, len(childPath)+1)
	path[0] = PathNode{KeyType: PathMap, FieldIndex: m.diffIndex, MapKey: key}
	copy(path[1:], childPath)
	m.parent.writePatch(path, operation, value)
}

package diff

import "github.com/2comjie/nova/generic"

type PointerMap[K generic.Primitive, V PointerValue] struct {
	_ noCopy

	values     map[K]V
	operations *mapOperationEntries[K]

	parent    *Object
	diffIndex uint32
	cleared   bool
}

func (m *PointerMap[K, V]) Init(parent *Object, diffIndex uint32) {
	m.parent = parent
	m.diffIndex = diffIndex
	m.operations = nil
	m.cleared = false

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
	value, ok := m.values[key]
	return value, ok
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

	m.recordOperation(key, MapOperationStore)
	m.parent.MarkDirty(m.diffIndex)
	return true
}

func (m *PointerMap[K, V]) Delete(key K) bool {
	value, exists := m.values[key]
	if !exists {
		return false
	}

	value.RemoveParent(m, key)
	delete(m.values, key)

	m.recordOperation(key, MapOperationDelete)
	m.parent.MarkDirty(m.diffIndex)
	return true
}

func (m *PointerMap[K, V]) Clear() bool {
	if len(m.values) == 0 && m.operations == nil {
		return false
	}

	for key, value := range m.values {
		value.RemoveParent(m, key)
	}

	m.values = nil
	m.operations = nil
	m.cleared = true
	m.parent.MarkDirty(m.diffIndex)
	return true
}

func (m *PointerMap[K, V]) Range(fn func(K, V) bool) {
	for key, value := range m.values {
		if !fn(key, value) {
			return
		}
	}
}

func (m *PointerMap[K, V]) MarkChildDirty(key any) {
	mapKey := key.(K)
	m.recordOperation(mapKey, MapOperationPatch)
	m.parent.MarkDirty(m.diffIndex)
}

func (m *PointerMap[K, V]) IsCleared() bool {
	return m.cleared
}

func (m *PointerMap[K, V]) RangeOperations(fn func(K, MapOperation) bool) {
	if m.operations == nil {
		return
	}

	for _, entry := range *m.operations {
		if !fn(entry.key, entry.operation) {
			return
		}
	}
}

func (m *PointerMap[K, V]) ClearDirty() {
	m.operations = nil
	m.cleared = false
}

func (m *PointerMap[K, V]) recordOperation(key K, operation MapOperation) {
	if m.operations == nil {
		entries := make(mapOperationEntries[K], 0, 1)
		m.operations = &entries
	}

	m.operations.record(key, operation)
}

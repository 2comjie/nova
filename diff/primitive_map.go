package diff

import "github.com/2comjie/nova/generic"

type PrimitiveMap[K generic.Primitive, V generic.Primitive] struct {
	_ noCopy

	values     map[K]V
	operations *mapOperationEntries[K]

	parent    *Object // 父对象
	diffIndex uint32
	cleared   bool
}

func (m *PrimitiveMap[K, V]) Init(parent *Object, diffIndex uint32) {
	m.parent = parent
	m.diffIndex = diffIndex
	m.operations = nil
	m.cleared = false
}

func (m *PrimitiveMap[K, V]) Len() int {
	return len(m.values)
}

func (m *PrimitiveMap[K, V]) Load(k K) (V, bool) {
	v, ok := m.values[k]
	return v, ok
}

func (m *PrimitiveMap[K, V]) Store(k K, v V) bool {
	old, ex := m.values[k]
	if ex && old == v {
		return false
	}

	if m.values == nil {
		m.values = make(map[K]V)
	}
	m.values[k] = v
	m.recordOperation(k, MapOperationStore) // store 操作
	m.parent.MarkDirty(m.diffIndex)
	return true
}

func (m *PrimitiveMap[K, V]) Delete(k K) bool {
	_, ex := m.values[k]
	if !ex {
		return false
	}

	delete(m.values, k)
	m.recordOperation(k, MapOperationDelete)
	m.parent.MarkDirty(m.diffIndex)
	return true
}

func (m *PrimitiveMap[K, V]) Clear() bool {
	if len(m.values) == 0 && m.operations == nil {
		return false
	}

	m.values = nil
	m.operations = nil
	m.cleared = true
	m.parent.MarkDirty(m.diffIndex)
	return true
}

func (m *PrimitiveMap[K, V]) Range(f func(K, V) bool) {
	if len(m.values) == 0 && m.operations == nil {
		return
	}
	for k, v := range m.values {
		if !f(k, v) {
			return
		}
	}
}

func (m *PrimitiveMap[K, V]) IsCleared() bool {
	return m.cleared
}

func (m *PrimitiveMap[K, V]) RangeOperations(fn func(key K, operation MapOperation) bool) {
	if m.operations == nil {
		return
	}

	for _, entry := range *m.operations {
		if !fn(entry.key, entry.operation) {
			return
		}
	}
}

func (m *PrimitiveMap[K, V]) ClearDirty() {
	m.operations = nil
	m.cleared = false
}

func (m *PrimitiveMap[K, V]) recordOperation(key K, operation MapOperation) {
	if m.operations == nil {
		entries := make(mapOperationEntries[K], 0, 1)
		m.operations = &entries
	}

	m.operations.record(key, operation)
}

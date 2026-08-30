package diff

import "reflect"

type ChangeOperation uint8

const (
	ChangeSet   ChangeOperation = 1
	ChangeClear ChangeOperation = 2

	ChangeMapStore  ChangeOperation = 3
	ChangeMapDelete ChangeOperation = 4
	ChangeMapClear  ChangeOperation = 5

	ChangeSliceSet    ChangeOperation = 6
	ChangeSliceAppend ChangeOperation = 7
	ChangeSliceInsert ChangeOperation = 8
	ChangeSliceDelete ChangeOperation = 9
	ChangeSliceMove   ChangeOperation = 10
	ChangeSliceClear  ChangeOperation = 11
)

type Change[T any] struct {
	Operation ChangeOperation
	Path      RuntimePath
	OldValue  T
	NewValue  T

	canceled     bool
	cancelReason string
}

func (c *Change[T]) Replace(value T) {
	c.NewValue = value
}

func (c *Change[T]) Cancel(reason ...string) {
	c.canceled = true
	if len(reason) == 1 {
		c.cancelReason = reason[0]
	}
}

func (c *Change[T]) Canceled() bool {
	return c.canceled
}

func (c *Change[T]) CancelReason() string {
	return c.cancelReason
}

type MapChange[K comparable, V any] struct {
	Operation ChangeOperation
	Path      RuntimePath
	Key       K
	HasKey    bool
	OldValue  V
	OldExists bool
	NewValue  V
	NewExists bool

	canceled     bool
	cancelReason string
}

func (c *MapChange[K, V]) Replace(value V) {
	c.NewValue = value
	c.NewExists = true
}

func (c *MapChange[K, V]) Cancel(reason ...string) {
	c.canceled = true
	if len(reason) == 1 {
		c.cancelReason = reason[0]
	}
}

func (c *MapChange[K, V]) Canceled() bool {
	return c.canceled
}

func (c *MapChange[K, V]) CancelReason() string {
	return c.cancelReason
}

type SliceChange[T any] struct {
	Operation ChangeOperation
	Path      RuntimePath
	Index     int
	ToIndex   int
	OldValue  T
	NewValue  T
	HasOld    bool
	HasNew    bool

	canceled     bool
	cancelReason string
}

func (c *SliceChange[T]) Replace(value T) {
	c.NewValue = value
	c.HasNew = true
}

func (c *SliceChange[T]) Cancel(reason ...string) {
	c.canceled = true
	if len(reason) == 1 {
		c.cancelReason = reason[0]
	}
}

func (c *SliceChange[T]) Canceled() bool {
	return c.canceled
}

func (c *SliceChange[T]) CancelReason() string {
	return c.cancelReason
}

type listenerSelector uint8

const (
	listenerField         listenerSelector = 1
	listenerMapKey        listenerSelector = 2
	listenerMapAny        listenerSelector = 3
	listenerMapCollection listenerSelector = 4
	listenerSliceIndex    listenerSelector = 5
	listenerSliceAny      listenerSelector = 6
	listenerSliceField    listenerSelector = 7
)

type listenerPathNode struct {
	selector   listenerSelector
	fieldIndex uint32
	key        any
	index      int
}

type listenerPath []listenerPathNode

type RuntimePath struct {
	nodes listenerPath
}

func (p RuntimePath) MapKey(fieldIndex uint32) (any, bool) {
	for _, node := range p.nodes {
		if node.selector == listenerMapKey && node.fieldIndex == fieldIndex {
			return node.key, true
		}
	}
	return nil, false
}

func (p RuntimePath) SliceIndex(fieldIndex uint32) (int, bool) {
	for _, node := range p.nodes {
		if node.selector == listenerSliceIndex && node.fieldIndex == fieldIndex {
			return node.index, true
		}
	}
	return 0, false
}

type PathBuilder[Root any] struct {
	nodes listenerPath
}

func NewPathBuilder[Root any]() PathBuilder[Root] {
	return PathBuilder[Root]{}
}

func (p PathBuilder[Root]) Field(diffIndex uint32) PathBuilder[Root] {
	return PathBuilder[Root]{nodes: appendListenerPath(p.nodes, listenerPathNode{selector: listenerField, fieldIndex: diffIndex})}
}

func (p PathBuilder[Root]) MapAny(diffIndex uint32) PathBuilder[Root] {
	return PathBuilder[Root]{nodes: appendListenerPath(p.nodes, listenerPathNode{selector: listenerMapAny, fieldIndex: diffIndex})}
}

func (p PathBuilder[Root]) MapKey(diffIndex uint32, key any) PathBuilder[Root] {
	return PathBuilder[Root]{nodes: appendListenerPath(p.nodes, listenerPathNode{selector: listenerMapKey, fieldIndex: diffIndex, key: key})}
}

func (p PathBuilder[Root]) SliceAny(diffIndex uint32) PathBuilder[Root] {
	return PathBuilder[Root]{nodes: appendListenerPath(p.nodes, listenerPathNode{selector: listenerSliceAny, fieldIndex: diffIndex})}
}

func (p PathBuilder[Root]) SliceIndex(diffIndex uint32, index int) PathBuilder[Root] {
	return PathBuilder[Root]{nodes: appendListenerPath(p.nodes, listenerPathNode{selector: listenerSliceIndex, fieldIndex: diffIndex, index: index})}
}

type ValuePath[Root any, Value any] struct {
	nodes listenerPath
}

func NewValuePath[Root any, Value any](path PathBuilder[Root]) ValuePath[Root, Value] {
	return ValuePath[Root, Value]{nodes: path.nodes}
}

type MapPath[Root any, Key comparable, Value any] struct {
	parent    listenerPath
	diffIndex uint32
	key       Key
	hasKey    bool
}

func NewMapPath[Root any, Key comparable, Value any](parent PathBuilder[Root], diffIndex uint32) MapPath[Root, Key, Value] {
	return MapPath[Root, Key, Value]{parent: parent.nodes, diffIndex: diffIndex}
}

func (p MapPath[Root, Key, Value]) Key(key Key) MapPath[Root, Key, Value] {
	p.key = key
	p.hasKey = true
	return p
}

func (p MapPath[Root, Key, Value]) AnyPath() PathBuilder[Root] {
	return PathBuilder[Root]{nodes: appendListenerPath(p.parent, listenerPathNode{selector: listenerMapAny, fieldIndex: p.diffIndex})}
}

func (p MapPath[Root, Key, Value]) KeyPath(key Key) PathBuilder[Root] {
	return PathBuilder[Root]{nodes: appendListenerPath(p.parent, listenerPathNode{selector: listenerMapKey, fieldIndex: p.diffIndex, key: key})}
}

func (p MapPath[Root, Key, Value]) listenerPath() listenerPath {
	selector := listenerMapCollection
	node := listenerPathNode{selector: selector, fieldIndex: p.diffIndex}
	if p.hasKey {
		node.selector = listenerMapKey
		node.key = p.key
	}
	return appendListenerPath(p.parent, node)
}

type SlicePath[Root any, Value any] struct {
	parent    listenerPath
	diffIndex uint32
	index     int
	hasIndex  bool
}

func NewSlicePath[Root any, Value any](parent PathBuilder[Root], diffIndex uint32) SlicePath[Root, Value] {
	return SlicePath[Root, Value]{parent: parent.nodes, diffIndex: diffIndex}
}

func (p SlicePath[Root, Value]) Index(index int) SlicePath[Root, Value] {
	p.index = index
	p.hasIndex = true
	return p
}

func (p SlicePath[Root, Value]) AnyPath() PathBuilder[Root] {
	return PathBuilder[Root]{nodes: appendListenerPath(p.parent, listenerPathNode{selector: listenerSliceAny, fieldIndex: p.diffIndex})}
}

func (p SlicePath[Root, Value]) IndexPath(index int) PathBuilder[Root] {
	return PathBuilder[Root]{nodes: appendListenerPath(p.parent, listenerPathNode{selector: listenerSliceIndex, fieldIndex: p.diffIndex, index: index})}
}

func (p SlicePath[Root, Value]) listenerPath() listenerPath {
	node := listenerPathNode{selector: listenerSliceField, fieldIndex: p.diffIndex}
	if p.hasIndex {
		node.selector = listenerSliceIndex
		node.index = p.index
	}
	return appendListenerPath(p.parent, node)
}

type listenerKind uint8

const (
	valueListener listenerKind = 1
	mapListener   listenerKind = 2
	sliceListener listenerKind = 3
)

type listenerPhase uint8

const (
	beforeListener listenerPhase = 1
	afterListener  listenerPhase = 2
)

type changeEvent struct {
	kind         listenerKind
	operation    ChangeOperation
	oldValue     any
	newValue     any
	key          any
	hasKey       bool
	oldExists    bool
	newExists    bool
	index        int
	toIndex      int
	hasOld       bool
	hasNew       bool
	canceled     bool
	cancelReason string
}

type listenerEntry struct {
	kind     listenerKind
	path     listenerPath
	callback func(*changeEvent, listenerPath)
}

type rootListeners struct {
	before []listenerEntry
	after  []listenerEntry
}

var globalListeners = make(map[reflect.Type]rootListeners)

func ListenBefore[Root any, Value any](path ValuePath[Root, Value], handler func(*Change[Value])) {
	registerListener(reflect.TypeFor[Root](), beforeListener, listenerEntry{
		kind: valueListener,
		path: path.nodes,
		callback: func(event *changeEvent, runtimePath listenerPath) {
			change := Change[Value]{
				Operation: event.operation,
				Path:      RuntimePath{nodes: runtimePath},
				OldValue:  event.oldValue.(Value),
				NewValue:  event.newValue.(Value),
			}
			handler(&change)
			event.newValue = change.NewValue
			event.canceled = change.canceled
			event.cancelReason = change.cancelReason
		},
	})
}

func ListenAfter[Root any, Value any](path ValuePath[Root, Value], handler func(Change[Value])) {
	registerListener(reflect.TypeFor[Root](), afterListener, listenerEntry{
		kind: valueListener,
		path: path.nodes,
		callback: func(event *changeEvent, runtimePath listenerPath) {
			handler(Change[Value]{
				Operation: event.operation,
				Path:      RuntimePath{nodes: runtimePath},
				OldValue:  event.oldValue.(Value),
				NewValue:  event.newValue.(Value),
			})
		},
	})
}

func ListenMapBefore[Root any, Key comparable, Value any](path MapPath[Root, Key, Value], handler func(*MapChange[Key, Value])) {
	registerListener(reflect.TypeFor[Root](), beforeListener, listenerEntry{
		kind: mapListener,
		path: path.listenerPath(),
		callback: func(event *changeEvent, runtimePath listenerPath) {
			change := MapChange[Key, Value]{
				Operation: event.operation,
				Path:      RuntimePath{nodes: runtimePath},
				Key:       event.key.(Key),
				HasKey:    event.hasKey,
				OldValue:  event.oldValue.(Value),
				OldExists: event.oldExists,
				NewValue:  event.newValue.(Value),
				NewExists: event.newExists,
			}
			handler(&change)
			event.newValue = change.NewValue
			event.newExists = change.NewExists
			event.canceled = change.canceled
			event.cancelReason = change.cancelReason
		},
	})
}

func ListenMapAfter[Root any, Key comparable, Value any](path MapPath[Root, Key, Value], handler func(MapChange[Key, Value])) {
	registerListener(reflect.TypeFor[Root](), afterListener, listenerEntry{
		kind: mapListener,
		path: path.listenerPath(),
		callback: func(event *changeEvent, runtimePath listenerPath) {
			handler(MapChange[Key, Value]{
				Operation: event.operation,
				Path:      RuntimePath{nodes: runtimePath},
				Key:       event.key.(Key),
				HasKey:    event.hasKey,
				OldValue:  event.oldValue.(Value),
				OldExists: event.oldExists,
				NewValue:  event.newValue.(Value),
				NewExists: event.newExists,
			})
		},
	})
}

func ListenSliceBefore[Root any, Value any](path SlicePath[Root, Value], handler func(*SliceChange[Value])) {
	registerListener(reflect.TypeFor[Root](), beforeListener, listenerEntry{
		kind: sliceListener,
		path: path.listenerPath(),
		callback: func(event *changeEvent, runtimePath listenerPath) {
			change := SliceChange[Value]{
				Operation: event.operation,
				Path:      RuntimePath{nodes: runtimePath},
				Index:     event.index,
				ToIndex:   event.toIndex,
				OldValue:  event.oldValue.(Value),
				NewValue:  event.newValue.(Value),
				HasOld:    event.hasOld,
				HasNew:    event.hasNew,
			}
			handler(&change)
			event.newValue = change.NewValue
			event.hasNew = change.HasNew
			event.canceled = change.canceled
			event.cancelReason = change.cancelReason
		},
	})
}

func ListenSliceAfter[Root any, Value any](path SlicePath[Root, Value], handler func(SliceChange[Value])) {
	registerListener(reflect.TypeFor[Root](), afterListener, listenerEntry{
		kind: sliceListener,
		path: path.listenerPath(),
		callback: func(event *changeEvent, runtimePath listenerPath) {
			handler(SliceChange[Value]{
				Operation: event.operation,
				Path:      RuntimePath{nodes: runtimePath},
				Index:     event.index,
				ToIndex:   event.toIndex,
				OldValue:  event.oldValue.(Value),
				NewValue:  event.newValue.(Value),
				HasOld:    event.hasOld,
				HasNew:    event.hasNew,
			})
		},
	})
}

func registerListener(rootType reflect.Type, phase listenerPhase, entry listenerEntry) {
	listeners := globalListeners[rootType]
	if phase == beforeListener {
		listeners.before = append(listeners.before, entry)
	} else {
		listeners.after = append(listeners.after, entry)
	}
	globalListeners[rootType] = listeners
}

func dispatchListeners(rootType reflect.Type, phase listenerPhase, path listenerPath, event *changeEvent) {
	listeners := globalListeners[rootType]
	entries := listeners.before
	if phase == afterListener {
		entries = listeners.after
	}
	for _, entry := range entries {
		if entry.kind != event.kind || !matchListenerPath(entry.path, path) {
			continue
		}
		entry.callback(event, path)
		if phase == beforeListener && event.canceled {
			return
		}
	}
}

func matchListenerPath(pattern listenerPath, path listenerPath) bool {
	if len(pattern) != len(path) {
		return false
	}
	for index, expected := range pattern {
		actual := path[index]
		if expected.fieldIndex != actual.fieldIndex {
			return false
		}
		switch expected.selector {
		case listenerField:
			if actual.selector != listenerField {
				return false
			}
		case listenerMapKey:
			if actual.selector != listenerMapKey || expected.key != actual.key {
				return false
			}
		case listenerMapAny:
			if actual.selector != listenerMapKey {
				return false
			}
		case listenerMapCollection:
			if actual.selector != listenerMapKey && actual.selector != listenerField {
				return false
			}
		case listenerSliceIndex:
			if actual.selector != listenerSliceIndex || expected.index != actual.index {
				return false
			}
		case listenerSliceAny:
			if actual.selector != listenerSliceIndex {
				return false
			}
		case listenerSliceField:
			if actual.selector != listenerField && actual.selector != listenerSliceIndex {
				return false
			}
		}
	}
	return true
}

func appendListenerPath(path listenerPath, node listenerPathNode) listenerPath {
	result := make(listenerPath, len(path)+1)
	copy(result, path)
	result[len(path)] = node
	return result
}

func prependListenerPath(node listenerPathNode, path listenerPath) listenerPath {
	result := make(listenerPath, len(path)+1)
	result[0] = node
	copy(result[1:], path)
	return result
}

func beforeValueChange[T any](parent *Object, diffIndex uint32, operation ChangeOperation, oldValue T, newValue T) (T, *changeEvent, bool) {
	if len(globalListeners) == 0 {
		return newValue, nil, true
	}
	event := &changeEvent{
		kind:      valueListener,
		operation: operation,
		oldValue:  oldValue,
		newValue:  newValue,
	}
	parent.dispatchChildChange(diffIndex, nil, beforeListener, event)
	return event.newValue.(T), event, !event.canceled
}

func afterValueChange(parent *Object, diffIndex uint32, event *changeEvent) {
	if event == nil {
		return
	}
	parent.dispatchChildChange(diffIndex, nil, afterListener, event)
}

func beforeMapChange[K comparable, V any](parent *Object, diffIndex uint32, operation ChangeOperation, key K, hasKey bool, oldValue V, oldExists bool, newValue V, newExists bool) (V, *changeEvent, bool) {
	if len(globalListeners) == 0 {
		return newValue, nil, true
	}
	event := &changeEvent{
		kind:      mapListener,
		operation: operation,
		key:       key,
		hasKey:    hasKey,
		oldValue:  oldValue,
		oldExists: oldExists,
		newValue:  newValue,
		newExists: newExists,
	}
	path := listenerPath{{selector: listenerField, fieldIndex: diffIndex}}
	if hasKey {
		path[0].selector = listenerMapKey
		path[0].key = key
	}
	parent.dispatchChange(path, beforeListener, event)
	return event.newValue.(V), event, !event.canceled
}

func afterMapChange(parent *Object, diffIndex uint32, event *changeEvent) {
	if event == nil {
		return
	}
	path := listenerPath{{selector: listenerField, fieldIndex: diffIndex}}
	if event.hasKey {
		path[0].selector = listenerMapKey
		path[0].key = event.key
	}
	parent.dispatchChange(path, afterListener, event)
}

func beforeSliceChange[T any](parent *Object, diffIndex uint32, operation ChangeOperation, index int, toIndex int, oldValue T, hasOld bool, newValue T, hasNew bool) (T, *changeEvent, bool) {
	if len(globalListeners) == 0 {
		return newValue, nil, true
	}
	event := &changeEvent{
		kind:      sliceListener,
		operation: operation,
		index:     index,
		toIndex:   toIndex,
		oldValue:  oldValue,
		newValue:  newValue,
		hasOld:    hasOld,
		hasNew:    hasNew,
	}
	path := listenerPath{{selector: listenerSliceIndex, fieldIndex: diffIndex, index: index}}
	if operation == ChangeSliceClear {
		path[0].selector = listenerField
	}
	parent.dispatchChange(path, beforeListener, event)
	return event.newValue.(T), event, !event.canceled
}

func afterSliceChange(parent *Object, diffIndex uint32, event *changeEvent) {
	if event == nil {
		return
	}
	path := listenerPath{{selector: listenerSliceIndex, fieldIndex: diffIndex, index: event.index}}
	if event.operation == ChangeSliceClear {
		path[0].selector = listenerField
	}
	parent.dispatchChange(path, afterListener, event)
}

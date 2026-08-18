package diff_test

import (
	"slices"
	"testing"

	. "github.com/2comjie/nova/diff"
	"github.com/2comjie/nova/diff/testdata"
	"google.golang.org/protobuf/proto"
)

const (
	playerStateDirtyBagWord   uint32 = 0
	playerStateDirtyBagMask   uint64 = 1
	playerStateDirtyBag2Word  uint32 = 0
	playerStateDirtyBag2Mask  uint64 = 2
	bagStateDirtyCapacityWord uint32 = 0
	bagStateDirtyCapacityMask uint64 = 1
	bagStateDirtyItemsWord    uint32 = 0
	bagStateDirtyItemsMask    uint64 = 2
	bagStateDirtyOrderWord    uint32 = 0
	bagStateDirtyOrderMask    uint64 = 4
	itemStateDirtyCountWord   uint32 = 0
	itemStateDirtyCountMask   uint64 = 1
)

type itemPatch struct {
	dirty [1]uint64
}

type itemMapChange[K comparable] struct {
	key       K
	operation Operation
	patch     itemPatch
}

type itemMapTracker[K comparable] struct {
	indexes map[K]int
	changes []itemMapChange[K]
}

type listChange[T any] struct {
	operation Operation
	index     uint32
	to        uint32
	value     T
}

type listTracker[T any] struct {
	changes []listChange[T]
}

type BagItemRef struct {
	value  *testdata.Item
	key    uint64
	parent *BagState
}

type BagItems struct {
	state *BagState
}

type BagOrder struct {
	state *BagState
}

type Bag2ItemRef struct {
	value  *testdata.Item
	key    uint64
	parent *Bag2State
}

type Bag2Items struct {
	state *Bag2State
}

type Bag2Order struct {
	state *Bag2State
}

type PlayerState struct {
	value         *testdata.Player
	dirty         [1]uint64
	bag           *BagState
	bagOperation  Operation
	bag2          *Bag2State
	bag2Operation Operation
}

type BagState struct {
	value        *testdata.Bag
	parent       *PlayerState
	dirty        [1]uint64
	itemChanges  itemMapTracker[uint64]
	orderChanges listTracker[uint64]
}

type Bag2State struct {
	value        *testdata.Bag2
	parent       *PlayerState
	dirty        [1]uint64
	itemChanges  itemMapTracker[uint64]
	orderChanges listTracker[uint64]
}

func (t *itemMapTracker[K]) Patch(key K) (*itemPatch, bool) {
	if index := t.indexes[key]; index != 0 {
		change := &t.changes[index-1]
		if change.operation != OperationMapPatch {
			return nil, false
		}
		return &change.patch, false
	}
	if t.indexes == nil {
		t.indexes = make(map[K]int)
	}
	first := len(t.changes) == 0
	t.changes = append(t.changes, itemMapChange[K]{key: key, operation: OperationMapPatch})
	t.indexes[key] = len(t.changes)
	return &t.changes[len(t.changes)-1].patch, first
}

func (t *itemMapTracker[K]) Put(key K) bool {
	if index := t.indexes[key]; index != 0 {
		change := &t.changes[index-1]
		change.operation = OperationMapPut
		change.patch = itemPatch{}
		return false
	}
	if t.indexes == nil {
		t.indexes = make(map[K]int)
	}
	first := len(t.changes) == 0
	t.changes = append(t.changes, itemMapChange[K]{key: key, operation: OperationMapPut})
	t.indexes[key] = len(t.changes)
	return first
}

func (t *itemMapTracker[K]) Delete(key K) bool {
	if index := t.indexes[key]; index != 0 {
		change := &t.changes[index-1]
		change.operation = OperationMapDelete
		change.patch = itemPatch{}
		return false
	}
	if t.indexes == nil {
		t.indexes = make(map[K]int)
	}
	first := len(t.changes) == 0
	t.changes = append(t.changes, itemMapChange[K]{key: key, operation: OperationMapDelete})
	t.indexes[key] = len(t.changes)
	return first
}

func (t *itemMapTracker[K]) ClearDirty() {
	clear(t.indexes)
	clear(t.changes)
	t.changes = t.changes[:0]
}

func (t *listTracker[T]) Append(value T) bool {
	first := len(t.changes) == 0
	t.changes = append(t.changes, listChange[T]{operation: OperationListAppend, value: value})
	return first
}

func (t *listTracker[T]) Insert(index uint32, value T) bool {
	first := len(t.changes) == 0
	t.changes = append(t.changes, listChange[T]{operation: OperationListInsert, index: index, value: value})
	return first
}

func (t *listTracker[T]) Store(index uint32, value T) bool {
	first := len(t.changes) == 0
	t.changes = append(t.changes, listChange[T]{operation: OperationListSet, index: index, value: value})
	return first
}

func (t *listTracker[T]) Delete(index uint32) bool {
	first := len(t.changes) == 0
	t.changes = append(t.changes, listChange[T]{operation: OperationListDelete, index: index})
	return first
}

func (t *listTracker[T]) Move(from uint32, to uint32) bool {
	first := len(t.changes) == 0
	t.changes = append(t.changes, listChange[T]{operation: OperationListMove, index: from, to: to})
	return first
}

func (t *listTracker[T]) ClearDirty() {
	clear(t.changes)
	t.changes = t.changes[:0]
}

func (r BagItemRef) GetCount() int32 {
	return r.value.Count
}

func (r BagItemRef) SetCount(value int32) {
	if r.value.Count == value {
		return
	}
	r.value.Count = value
	patch, first := r.parent.itemChanges.Patch(r.key)
	if patch != nil {
		MarkDirty(&patch.dirty[itemStateDirtyCountWord], itemStateDirtyCountMask)
	}
	if first {
		r.parent.markItemsDirty()
	}
}

func (r Bag2ItemRef) GetCount() int32 {
	return r.value.Count
}

func (r Bag2ItemRef) SetCount(value int32) {
	if r.value.Count == value {
		return
	}
	r.value.Count = value
	patch, first := r.parent.itemChanges.Patch(r.key)
	if patch != nil {
		MarkDirty(&patch.dirty[itemStateDirtyCountWord], itemStateDirtyCountMask)
	}
	if first {
		r.parent.markItemsDirty()
	}
}

func NewPlayerState(value *testdata.Player) *PlayerState {
	state := &PlayerState{value: value}
	if value.Bag != nil {
		state.bag = &BagState{value: value.Bag, parent: state}
	}
	if value.Bag2 != nil {
		state.bag2 = &Bag2State{value: value.Bag2, parent: state}
	}
	return state
}

func (s *PlayerState) markBagDirty() {
	if MarkDirty(&s.dirty[playerStateDirtyBagWord], playerStateDirtyBagMask) {
		s.bagOperation = OperationPatch
	}
}

func (s *PlayerState) markBag2Dirty() {
	if MarkDirty(&s.dirty[playerStateDirtyBag2Word], playerStateDirtyBag2Mask) {
		s.bag2Operation = OperationPatch
	}
}

func (s *BagState) markItemsDirty() {
	if MarkDirty(&s.dirty[bagStateDirtyItemsWord], bagStateDirtyItemsMask) {
		s.parent.markBagDirty()
	}
}

func (s *BagState) markOrderDirty() {
	if MarkDirty(&s.dirty[bagStateDirtyOrderWord], bagStateDirtyOrderMask) {
		s.parent.markBagDirty()
	}
}

func (s *BagState) markCapacityDirty() {
	if MarkDirty(&s.dirty[bagStateDirtyCapacityWord], bagStateDirtyCapacityMask) {
		s.parent.markBagDirty()
	}
}

func (s *Bag2State) markItemsDirty() {
	if MarkDirty(&s.dirty[bagStateDirtyItemsWord], bagStateDirtyItemsMask) {
		s.parent.markBag2Dirty()
	}
}

func (s *Bag2State) markOrderDirty() {
	if MarkDirty(&s.dirty[bagStateDirtyOrderWord], bagStateDirtyOrderMask) {
		s.parent.markBag2Dirty()
	}
}

func (s *Bag2State) markCapacityDirty() {
	if MarkDirty(&s.dirty[bagStateDirtyCapacityWord], bagStateDirtyCapacityMask) {
		s.parent.markBag2Dirty()
	}
}

func (s *PlayerState) GetBag() *BagState {
	return s.bag
}

func (s *PlayerState) SetBag(value *testdata.Bag) {
	s.value.Bag = value
	s.bag = &BagState{value: value, parent: s}
	s.bagOperation = OperationReplace
	MarkDirty(&s.dirty[playerStateDirtyBagWord], playerStateDirtyBagMask)
}

func (s *PlayerState) ClearBag() {
	if s.bag == nil {
		return
	}
	s.bag = nil
	s.value.Bag = nil
	s.bagOperation = OperationClear
	MarkDirty(&s.dirty[playerStateDirtyBagWord], playerStateDirtyBagMask)
}

func (s *PlayerState) GetBag2() *Bag2State {
	return s.bag2
}

func (s *PlayerState) SetBag2(value *testdata.Bag2) {
	s.value.Bag2 = value
	s.bag2 = &Bag2State{value: value, parent: s}
	s.bag2Operation = OperationReplace
	MarkDirty(&s.dirty[playerStateDirtyBag2Word], playerStateDirtyBag2Mask)
}

func (s *PlayerState) ClearBag2() {
	if s.bag2 == nil {
		return
	}
	s.bag2 = nil
	s.value.Bag2 = nil
	s.bag2Operation = OperationClear
	MarkDirty(&s.dirty[playerStateDirtyBag2Word], playerStateDirtyBag2Mask)
}

func (s *BagState) GetCapacity() int32 {
	return s.value.Capacity
}

func (s *BagState) SetCapacity(value int32) {
	if s.value.Capacity == value {
		return
	}
	s.value.Capacity = value
	s.markCapacityDirty()
}

func (s *Bag2State) GetCapacity() int32 {
	return s.value.Capacity
}

func (s *Bag2State) SetCapacity(value int32) {
	if s.value.Capacity == value {
		return
	}
	s.value.Capacity = value
	s.markCapacityDirty()
}

func (s *BagState) Items() BagItems {
	return BagItems{state: s}
}

func (m BagItems) GetValue(key uint64) (BagItemRef, bool) {
	value, ok := m.state.value.Items[key]
	return BagItemRef{value: value, key: key, parent: m.state}, ok
}

func (m BagItems) Store(key uint64, value *testdata.Item) {
	if m.state.value.Items == nil {
		m.state.value.Items = make(map[uint64]*testdata.Item)
	}
	m.state.value.Items[key] = value
	if m.state.itemChanges.Put(key) {
		m.state.markItemsDirty()
	}
}

func (m BagItems) Delete(key uint64) {
	if _, ok := m.state.value.Items[key]; !ok {
		return
	}
	delete(m.state.value.Items, key)
	if m.state.itemChanges.Delete(key) {
		m.state.markItemsDirty()
	}
}

func (m BagItems) Range(yield func(uint64, BagItemRef) bool) {
	for key, value := range m.state.value.Items {
		if !yield(key, BagItemRef{value: value, key: key, parent: m.state}) {
			return
		}
	}
}

func (s *BagState) Order() BagOrder {
	return BagOrder{state: s}
}

func (l BagOrder) Len() int {
	return len(l.state.value.Order)
}

func (l BagOrder) GetValue(index int) (uint64, bool) {
	if index < 0 || index >= len(l.state.value.Order) {
		return 0, false
	}
	return l.state.value.Order[index], true
}

func (l BagOrder) Store(index int, value uint64) {
	if l.state.value.Order[index] == value {
		return
	}
	l.state.value.Order[index] = value
	if l.state.orderChanges.Store(uint32(index), value) {
		l.state.markOrderDirty()
	}
}

func (l BagOrder) Append(value uint64) {
	l.state.value.Order = append(l.state.value.Order, value)
	if l.state.orderChanges.Append(value) {
		l.state.markOrderDirty()
	}
}

func (l BagOrder) Insert(index int, value uint64) {
	l.state.value.Order = append(l.state.value.Order, 0)
	copy(l.state.value.Order[index+1:], l.state.value.Order[index:len(l.state.value.Order)-1])
	l.state.value.Order[index] = value
	if l.state.orderChanges.Insert(uint32(index), value) {
		l.state.markOrderDirty()
	}
}

func (l BagOrder) Delete(index int) {
	copy(l.state.value.Order[index:], l.state.value.Order[index+1:])
	l.state.value.Order = l.state.value.Order[:len(l.state.value.Order)-1]
	if l.state.orderChanges.Delete(uint32(index)) {
		l.state.markOrderDirty()
	}
}

func (l BagOrder) Move(from int, to int) {
	if from == to {
		return
	}
	value := l.state.value.Order[from]
	if from < to {
		copy(l.state.value.Order[from:to], l.state.value.Order[from+1:to+1])
	} else {
		copy(l.state.value.Order[to+1:from+1], l.state.value.Order[to:from])
	}
	l.state.value.Order[to] = value
	if l.state.orderChanges.Move(uint32(from), uint32(to)) {
		l.state.markOrderDirty()
	}
}

func (l BagOrder) Range(yield func(int, uint64) bool) {
	for index, value := range l.state.value.Order {
		if !yield(index, value) {
			return
		}
	}
}

func (s *Bag2State) Items() Bag2Items {
	return Bag2Items{state: s}
}

func (m Bag2Items) GetValue(key uint64) (Bag2ItemRef, bool) {
	value, ok := m.state.value.Items[key]
	return Bag2ItemRef{value: value, key: key, parent: m.state}, ok
}

func (m Bag2Items) Store(key uint64, value *testdata.Item) {
	if m.state.value.Items == nil {
		m.state.value.Items = make(map[uint64]*testdata.Item)
	}
	m.state.value.Items[key] = value
	if m.state.itemChanges.Put(key) {
		m.state.markItemsDirty()
	}
}

func (m Bag2Items) Delete(key uint64) {
	if _, ok := m.state.value.Items[key]; !ok {
		return
	}
	delete(m.state.value.Items, key)
	if m.state.itemChanges.Delete(key) {
		m.state.markItemsDirty()
	}
}

func (m Bag2Items) Range(yield func(uint64, Bag2ItemRef) bool) {
	for key, value := range m.state.value.Items {
		if !yield(key, Bag2ItemRef{value: value, key: key, parent: m.state}) {
			return
		}
	}
}

func (s *Bag2State) Order() Bag2Order {
	return Bag2Order{state: s}
}

func (l Bag2Order) Len() int {
	return len(l.state.value.Order)
}

func (l Bag2Order) GetValue(index int) (uint64, bool) {
	if index < 0 || index >= len(l.state.value.Order) {
		return 0, false
	}
	return l.state.value.Order[index], true
}

func (l Bag2Order) Store(index int, value uint64) {
	if l.state.value.Order[index] == value {
		return
	}
	l.state.value.Order[index] = value
	if l.state.orderChanges.Store(uint32(index), value) {
		l.state.markOrderDirty()
	}
}

func (l Bag2Order) Append(value uint64) {
	l.state.value.Order = append(l.state.value.Order, value)
	if l.state.orderChanges.Append(value) {
		l.state.markOrderDirty()
	}
}

func (l Bag2Order) Insert(index int, value uint64) {
	l.state.value.Order = append(l.state.value.Order, 0)
	copy(l.state.value.Order[index+1:], l.state.value.Order[index:len(l.state.value.Order)-1])
	l.state.value.Order[index] = value
	if l.state.orderChanges.Insert(uint32(index), value) {
		l.state.markOrderDirty()
	}
}

func (l Bag2Order) Delete(index int) {
	copy(l.state.value.Order[index:], l.state.value.Order[index+1:])
	l.state.value.Order = l.state.value.Order[:len(l.state.value.Order)-1]
	if l.state.orderChanges.Delete(uint32(index)) {
		l.state.markOrderDirty()
	}
}

func (l Bag2Order) Move(from int, to int) {
	if from == to {
		return
	}
	value := l.state.value.Order[from]
	if from < to {
		copy(l.state.value.Order[from:to], l.state.value.Order[from+1:to+1])
	} else {
		copy(l.state.value.Order[to+1:from+1], l.state.value.Order[to:from])
	}
	l.state.value.Order[to] = value
	if l.state.orderChanges.Move(uint32(from), uint32(to)) {
		l.state.markOrderDirty()
	}
}

func (l Bag2Order) Range(yield func(int, uint64) bool) {
	for index, value := range l.state.value.Order {
		if !yield(index, value) {
			return
		}
	}
}

func (s *PlayerState) WriteDiff(writer *Writer) {
	if HasDirty(s.dirty[playerStateDirtyBagWord], playerStateDirtyBagMask) {
		switch s.bagOperation {
		case OperationPatch:
			writer.Patch(1, s.bag.WriteDiff)
		case OperationReplace:
			data, err := proto.Marshal(s.bag.value)
			if err != nil {
				panic(err)
			}
			writer.Replace(1, func(writer *Writer) {
				writer.AppendRaw(data)
			})
		case OperationClear:
			writer.Clear(1)
		}
	}
	if HasDirty(s.dirty[playerStateDirtyBag2Word], playerStateDirtyBag2Mask) {
		switch s.bag2Operation {
		case OperationPatch:
			writer.Patch(2, s.bag2.WriteDiff)
		case OperationReplace:
			data, err := proto.Marshal(s.bag2.value)
			if err != nil {
				panic(err)
			}
			writer.Replace(2, func(writer *Writer) {
				writer.AppendRaw(data)
			})
		case OperationClear:
			writer.Clear(2)
		}
	}
}

func (s *BagState) WriteDiff(writer *Writer) {
	if HasDirty(s.dirty[bagStateDirtyCapacityWord], bagStateDirtyCapacityMask) {
		writer.Int32(1, s.value.Capacity)
	}
	for index := range s.itemChanges.changes {
		change := &s.itemChanges.changes[index]
		switch change.operation {
		case OperationMapPut:
			data, err := proto.Marshal(s.value.Items[change.key])
			if err != nil {
				panic(err)
			}
			writer.MapPut(2, func(writer *Writer) {
				writer.AppendUint64(change.key)
			}, func(writer *Writer) {
				writer.AppendBytes(data)
			})
		case OperationMapDelete:
			writer.MapDelete(2, func(writer *Writer) {
				writer.AppendUint64(change.key)
			})
		case OperationMapPatch:
			writer.MapPatch(2, func(writer *Writer) {
				writer.AppendUint64(change.key)
			}, func(writer *Writer) {
				writeItemPatch(writer, s.value.Items[change.key], &change.patch)
			})
		}
	}
	for _, change := range s.orderChanges.changes {
		switch change.operation {
		case OperationListAppend:
			writer.ListAppend(3, func(writer *Writer) {
				writer.AppendUint64(change.value)
			})
		case OperationListInsert:
			writer.ListInsert(3, change.index, func(writer *Writer) {
				writer.AppendUint64(change.value)
			})
		case OperationListSet:
			writer.ListSet(3, change.index, func(writer *Writer) {
				writer.AppendUint64(change.value)
			})
		case OperationListDelete:
			writer.ListDelete(3, change.index)
		case OperationListMove:
			writer.ListMove(3, change.index, change.to)
		}
	}
}

func (s *Bag2State) WriteDiff(writer *Writer) {
	if HasDirty(s.dirty[bagStateDirtyCapacityWord], bagStateDirtyCapacityMask) {
		writer.Int32(1, s.value.Capacity)
	}
	for index := range s.itemChanges.changes {
		change := &s.itemChanges.changes[index]
		switch change.operation {
		case OperationMapPut:
			data, err := proto.Marshal(s.value.Items[change.key])
			if err != nil {
				panic(err)
			}
			writer.MapPut(2, func(writer *Writer) {
				writer.AppendUint64(change.key)
			}, func(writer *Writer) {
				writer.AppendBytes(data)
			})
		case OperationMapDelete:
			writer.MapDelete(2, func(writer *Writer) {
				writer.AppendUint64(change.key)
			})
		case OperationMapPatch:
			writer.MapPatch(2, func(writer *Writer) {
				writer.AppendUint64(change.key)
			}, func(writer *Writer) {
				writeItemPatch(writer, s.value.Items[change.key], &change.patch)
			})
		}
	}
	for _, change := range s.orderChanges.changes {
		switch change.operation {
		case OperationListAppend:
			writer.ListAppend(3, func(writer *Writer) {
				writer.AppendUint64(change.value)
			})
		case OperationListInsert:
			writer.ListInsert(3, change.index, func(writer *Writer) {
				writer.AppendUint64(change.value)
			})
		case OperationListSet:
			writer.ListSet(3, change.index, func(writer *Writer) {
				writer.AppendUint64(change.value)
			})
		case OperationListDelete:
			writer.ListDelete(3, change.index)
		case OperationListMove:
			writer.ListMove(3, change.index, change.to)
		}
	}
}

func writeItemPatch(writer *Writer, item *testdata.Item, patch *itemPatch) {
	if HasDirty(patch.dirty[itemStateDirtyCountWord], itemStateDirtyCountMask) {
		writer.Int32(2, item.Count)
	}
}

func (s *PlayerState) ClearDirty() {
	ClearDirty(s.dirty[:])
	s.bagOperation = 0
	s.bag2Operation = 0
	if s.bag != nil {
		s.bag.ClearDirty()
	}
	if s.bag2 != nil {
		s.bag2.ClearDirty()
	}
}

func (s *BagState) ClearDirty() {
	ClearDirty(s.dirty[:])
	s.itemChanges.ClearDirty()
	s.orderChanges.ClearDirty()
}

func (s *Bag2State) ClearDirty() {
	ClearDirty(s.dirty[:])
	s.itemChanges.ClearDirty()
	s.orderChanges.ClearDirty()
}

func TestMapRefOnlyTracksChangedKeys(t *testing.T) {
	original := &testdata.Player{
		Bag: &testdata.Bag{Items: map[uint64]*testdata.Item{
			1001: {Id: 1001, Count: 10},
			1002: {Id: 1002, Count: 20},
		}},
		Bag2: &testdata.Bag2{Items: map[uint64]*testdata.Item{
			2001: {Id: 2001, Count: 30},
			2002: {Id: 2002, Count: 40},
		}},
	}
	replica := proto.Clone(original).(*testdata.Player)
	state := NewPlayerState(original)
	bag := state.GetBag()
	bag2 := state.GetBag2()
	items := bag.Items()
	items2 := bag2.Items()

	_, _ = items.GetValue(1002)
	_, _ = items2.GetValue(2002)
	if len(state.bag.itemChanges.changes) != 0 || len(state.bag2.itemChanges.changes) != 0 {
		t.Fatal("reading items must not create changes")
	}

	bagItem, _ := items.GetValue(1001)
	bagItem.SetCount(11)
	bagItem.SetCount(12)
	bag2Item, _ := items2.GetValue(2001)
	bag2Item.SetCount(31)

	if len(state.bag.itemChanges.changes) != 1 || len(state.bag2.itemChanges.changes) != 1 {
		t.Fatalf("expected one change per bag, got bag=%d bag2=%d", len(state.bag.itemChanges.changes), len(state.bag2.itemChanges.changes))
	}
	if !HasDirty(state.dirty[playerStateDirtyBagWord], playerStateDirtyBagMask) || !HasDirty(state.dirty[playerStateDirtyBag2Word], playerStateDirtyBag2Mask) {
		t.Fatal("bag and bag2 should bubble to player")
	}

	writer := NewWriter(nil)
	state.WriteDiff(writer)
	if err := applyTrackedPlayerDiff(replica, writer.Data()); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(replica, original) {
		t.Fatalf("expected %v, got %v", original, replica)
	}

	state.ClearDirty()
	if AnyDirty(state.dirty[:]) || AnyDirty(state.bag.dirty[:]) || AnyDirty(state.bag2.dirty[:]) {
		t.Fatal("state should be clean")
	}
	if len(state.bag.itemChanges.changes) != 0 || len(state.bag2.itemChanges.changes) != 0 {
		t.Fatal("changes should be empty")
	}

	bagItem, _ = items.GetValue(1001)
	bagItem.SetCount(13)
	if len(state.bag.itemChanges.changes) != 1 || !HasDirty(state.dirty[playerStateDirtyBagWord], playerStateDirtyBagMask) {
		t.Fatal("change should bubble again after clear")
	}
}

func TestMapOperationsAndMerge(t *testing.T) {
	original := &testdata.Player{
		Bag: &testdata.Bag{Items: map[uint64]*testdata.Item{
			1001: {Id: 1001, Count: 10},
			1002: {Id: 1002, Count: 20},
			1003: {Id: 1003, Count: 30},
		}},
		Bag2: &testdata.Bag2{Items: map[uint64]*testdata.Item{
			4001: {Id: 4001, Count: 40},
		}},
	}
	replica := proto.Clone(original).(*testdata.Player)
	state := NewPlayerState(original)
	bag := state.GetBag()
	bag2 := state.GetBag2()
	items := bag.Items()
	items2 := bag2.Items()

	item, ok := items.GetValue(1001)
	if !ok || item.GetCount() != 10 {
		t.Fatal("load existing item failed")
	}
	item.SetCount(11)
	items.Store(1001, &testdata.Item{Id: 1001, Count: 12})

	items.Store(2001, &testdata.Item{Id: 2001, Count: 1})
	item, _ = items.GetValue(2001)
	item.SetCount(2)

	item, _ = items.GetValue(1002)
	item.SetCount(21)
	items.Delete(1002)

	items.Delete(1003)
	items.Store(1003, &testdata.Item{Id: 1003, Count: 31})
	items.Store(3001, &testdata.Item{Id: 3001, Count: 1})
	items.Delete(3001)
	items.Delete(9999)
	items2.Store(4002, &testdata.Item{Id: 4002, Count: 41})
	items2.Delete(4001)

	count := 0
	items.Range(func(_ uint64, _ BagItemRef) bool {
		count++
		return true
	})
	if count != 3 {
		t.Fatalf("expected 3 items, got %d", count)
	}

	expectedOperations := map[uint64]Operation{
		1001: OperationMapPut,
		2001: OperationMapPut,
		1002: OperationMapDelete,
		1003: OperationMapPut,
		3001: OperationMapDelete,
	}
	if len(state.bag.itemChanges.changes) != len(expectedOperations) {
		t.Fatalf("expected %d changes, got %d", len(expectedOperations), len(state.bag.itemChanges.changes))
	}
	for _, change := range state.bag.itemChanges.changes {
		if expectedOperations[change.key] != change.operation {
			t.Fatalf("key %d expected operation %d, got %d", change.key, expectedOperations[change.key], change.operation)
		}
	}

	writer := NewWriter(nil)
	state.WriteDiff(writer)
	if err := applyTrackedPlayerDiff(replica, writer.Data()); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(replica, original) {
		t.Fatalf("expected %v, got %v", original, replica)
	}
}

func TestListOperations(t *testing.T) {
	original := &testdata.Player{
		Bag:  &testdata.Bag{Order: []uint64{10, 20, 30}},
		Bag2: &testdata.Bag2{Order: []uint64{100, 200}},
	}
	replica := proto.Clone(original).(*testdata.Player)
	state := NewPlayerState(original)
	bag := state.GetBag()
	bag2 := state.GetBag2()
	order := bag.Order()
	order2 := bag2.Order()

	if value, ok := order.GetValue(1); !ok || value != 20 {
		t.Fatal("load existing index failed")
	}
	if _, ok := order.GetValue(3); ok {
		t.Fatal("load missing index should return false")
	}

	order.Store(1, 21)
	order.Append(40)
	order.Insert(1, 15)
	order.Delete(3)
	order.Move(3, 1)

	order2.Append(300)
	order2.Move(0, 2)
	order2.Store(1, 301)
	order2.Delete(0)
	order2.Insert(1, 150)

	if order.Len() != 4 || order2.Len() != 3 {
		t.Fatalf("unexpected lengths bag=%d bag2=%d", order.Len(), order2.Len())
	}
	values := make([]uint64, 0, order.Len())
	order.Range(func(_ int, value uint64) bool {
		values = append(values, value)
		return true
	})
	if !slices.Equal(values, []uint64{10, 40, 15, 21}) {
		t.Fatalf("unexpected order %v", values)
	}

	expectedOperations := []Operation{
		OperationListSet,
		OperationListAppend,
		OperationListInsert,
		OperationListDelete,
		OperationListMove,
	}
	if len(state.bag.orderChanges.changes) != len(expectedOperations) {
		t.Fatalf("expected %d changes, got %d", len(expectedOperations), len(state.bag.orderChanges.changes))
	}
	for index, operation := range expectedOperations {
		if state.bag.orderChanges.changes[index].operation != operation {
			t.Fatalf("change %d expected operation %d, got %d", index, operation, state.bag.orderChanges.changes[index].operation)
		}
	}

	writer := NewWriter(nil)
	state.WriteDiff(writer)
	if err := applyTrackedPlayerDiff(replica, writer.Data()); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(replica, original) {
		t.Fatalf("expected %v, got %v", original, replica)
	}

	state.ClearDirty()
	if len(state.bag.orderChanges.changes) != 0 || len(state.bag2.orderChanges.changes) != 0 {
		t.Fatal("list changes should be empty")
	}
}

func TestMessageAndScalarOperations(t *testing.T) {
	original := &testdata.Player{
		Bag:  &testdata.Bag{Capacity: 50},
		Bag2: &testdata.Bag2{Capacity: 60},
	}
	replica := proto.Clone(original).(*testdata.Player)
	state := NewPlayerState(original)

	bag := state.GetBag()
	if bag == nil || bag.GetCapacity() != 50 {
		t.Fatal("load bag failed")
	}
	bag.SetCapacity(51)
	bag2 := state.GetBag2()
	bag2.SetCapacity(61)
	if state.bagOperation != OperationPatch || state.bag2Operation != OperationPatch {
		t.Fatal("scalar changes should patch messages")
	}

	writer := NewWriter(nil)
	state.WriteDiff(writer)
	if err := applyTrackedPlayerDiff(replica, writer.Data()); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(replica, original) {
		t.Fatalf("expected %v, got %v", original, replica)
	}
	state.ClearDirty()

	state.SetBag(&testdata.Bag{Capacity: 100})
	bag = state.GetBag()
	bag.SetCapacity(101)
	bag2 = state.GetBag2()
	bag2.SetCapacity(62)
	state.ClearBag2()
	if state.bagOperation != OperationReplace || state.bag2Operation != OperationClear {
		t.Fatal("store and delete should override previous operations")
	}
	if state.GetBag2() != nil {
		t.Fatal("deleted bag2 should not load")
	}

	writer = NewWriter(nil)
	state.WriteDiff(writer)
	if err := applyTrackedPlayerDiff(replica, writer.Data()); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(replica, original) {
		t.Fatalf("expected %v, got %v", original, replica)
	}
	state.ClearDirty()

	state.ClearBag()
	state.SetBag(&testdata.Bag{Capacity: 200})
	state.SetBag2(&testdata.Bag2{Capacity: 300})
	if state.bagOperation != OperationReplace || state.bag2Operation != OperationReplace {
		t.Fatal("store after delete should replace message")
	}

	writer = NewWriter(nil)
	state.WriteDiff(writer)
	if err := applyTrackedPlayerDiff(replica, writer.Data()); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(replica, original) {
		t.Fatalf("expected %v, got %v", original, replica)
	}
}

func applyTrackedPlayerDiff(player *testdata.Player, data []byte) error {
	reader := NewReader(data)
	for {
		fieldNumber, operation, payload, ok, err := reader.Next()
		if err != nil || !ok {
			return err
		}
		switch fieldNumber {
		case 1:
			switch operation {
			case OperationPatch:
				if err := applyBagDiff(player.Bag, payload); err != nil {
					return err
				}
			case OperationReplace:
				player.Bag = &testdata.Bag{}
				if err := proto.Unmarshal(payload, player.Bag); err != nil {
					return err
				}
			case OperationClear:
				player.Bag = nil
			default:
				return ErrInvalidData
			}
		case 2:
			switch operation {
			case OperationPatch:
				if err := applyBag2Diff(player.Bag2, payload); err != nil {
					return err
				}
			case OperationReplace:
				player.Bag2 = &testdata.Bag2{}
				if err := proto.Unmarshal(payload, player.Bag2); err != nil {
					return err
				}
			case OperationClear:
				player.Bag2 = nil
			default:
				return ErrInvalidData
			}
		}
	}
}

func applyBag2Diff(bag *testdata.Bag2, data []byte) error {
	reader := NewReader(data)
	for {
		fieldNumber, operation, payload, ok, err := reader.Next()
		if err != nil || !ok {
			return err
		}
		valueReader := NewValueReader(payload)
		switch fieldNumber {
		case 1:
			if operation != OperationSetVarint {
				return ErrInvalidData
			}
			bag.Capacity = valueReader.Int32()
			if valueReader.Err() != nil || !valueReader.Done() {
				return ErrInvalidData
			}
		case 2:
			key := valueReader.Uint64()
			switch operation {
			case OperationMapPut:
				itemData := valueReader.Bytes()
				if valueReader.Err() != nil || !valueReader.Done() {
					return ErrInvalidData
				}
				item := &testdata.Item{}
				if err := proto.Unmarshal(itemData, item); err != nil {
					return err
				}
				if bag.Items == nil {
					bag.Items = make(map[uint64]*testdata.Item)
				}
				bag.Items[key] = item
			case OperationMapDelete:
				if valueReader.Err() != nil || !valueReader.Done() {
					return ErrInvalidData
				}
				delete(bag.Items, key)
			case OperationMapPatch:
				patch := valueReader.Remaining()
				if valueReader.Err() != nil {
					return ErrInvalidData
				}
				if err := applyItemDiff(bag.Items[key], patch); err != nil {
					return err
				}
			default:
				return ErrInvalidData
			}
		case 3:
			switch operation {
			case OperationListAppend:
				bag.Order = append(bag.Order, valueReader.Uint64())
			case OperationListInsert:
				index := int(valueReader.Uint32())
				value := valueReader.Uint64()
				bag.Order = append(bag.Order, 0)
				copy(bag.Order[index+1:], bag.Order[index:len(bag.Order)-1])
				bag.Order[index] = value
			case OperationListSet:
				index := int(valueReader.Uint32())
				bag.Order[index] = valueReader.Uint64()
			case OperationListDelete:
				index := int(valueReader.Uint32())
				copy(bag.Order[index:], bag.Order[index+1:])
				bag.Order = bag.Order[:len(bag.Order)-1]
			case OperationListMove:
				from := int(valueReader.Uint32())
				to := int(valueReader.Uint32())
				value := bag.Order[from]
				if from < to {
					copy(bag.Order[from:to], bag.Order[from+1:to+1])
				} else {
					copy(bag.Order[to+1:from+1], bag.Order[to:from])
				}
				bag.Order[to] = value
			default:
				return ErrInvalidData
			}
			if valueReader.Err() != nil || !valueReader.Done() {
				return ErrInvalidData
			}
		default:
			return ErrInvalidData
		}
	}
}

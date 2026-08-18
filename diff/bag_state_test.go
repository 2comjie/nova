package diff

import (
	"testing"

	"github.com/2comjie/nova/diff/testdata"
	"google.golang.org/protobuf/proto"
)

const (
	playerStateDirtyBagWord  uint32 = 0
	playerStateDirtyBagMask  uint64 = 1
	playerStateDirtyBag2Word uint32 = 0
	playerStateDirtyBag2Mask uint64 = 2
	bagStateDirtyItemsWord   uint32 = 0
	bagStateDirtyItemsMask   uint64 = 1
	itemStateDirtyCountWord  uint32 = 0
	itemStateDirtyCountMask  uint64 = 1
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

type BagItemRef struct {
	value  *testdata.Item
	key    uint64
	parent *BagState
}

type BagItems struct {
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

type PlayerState struct {
	dirty [1]uint64
	bag   *BagState
	bag2  *Bag2State
}

type BagState struct {
	value       *testdata.Bag
	parent      *PlayerState
	dirty       [1]uint64
	itemChanges itemMapTracker[uint64]
}

type Bag2State struct {
	value       *testdata.Bag2
	parent      *PlayerState
	dirty       [1]uint64
	itemChanges itemMapTracker[uint64]
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

func (r BagItemRef) LoadCount() int32 {
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

func (r Bag2ItemRef) LoadCount() int32 {
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
	state := &PlayerState{}
	state.bag = &BagState{value: value.Bag, parent: state}
	state.bag2 = &Bag2State{value: value.Bag2, parent: state}
	return state
}

func (s *PlayerState) markBagDirty() {
	MarkDirty(&s.dirty[playerStateDirtyBagWord], playerStateDirtyBagMask)
}

func (s *PlayerState) markBag2Dirty() {
	MarkDirty(&s.dirty[playerStateDirtyBag2Word], playerStateDirtyBag2Mask)
}

func (s *BagState) markItemsDirty() {
	if MarkDirty(&s.dirty[bagStateDirtyItemsWord], bagStateDirtyItemsMask) {
		s.parent.markBagDirty()
	}
}

func (s *Bag2State) markItemsDirty() {
	if MarkDirty(&s.dirty[bagStateDirtyItemsWord], bagStateDirtyItemsMask) {
		s.parent.markBag2Dirty()
	}
}

func (s *PlayerState) LoadBag() *BagState {
	return s.bag
}

func (s *PlayerState) LoadBag2() *Bag2State {
	return s.bag2
}

func (s *BagState) Items() BagItems {
	return BagItems{state: s}
}

func (m BagItems) Load(key uint64) (BagItemRef, bool) {
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

func (s *Bag2State) Items() Bag2Items {
	return Bag2Items{state: s}
}

func (m Bag2Items) Load(key uint64) (Bag2ItemRef, bool) {
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

func (s *PlayerState) WriteDiff(writer *Writer) {
	if HasDirty(s.dirty[playerStateDirtyBagWord], playerStateDirtyBagMask) {
		writer.Patch(1, s.bag.WriteDiff)
	}
	if HasDirty(s.dirty[playerStateDirtyBag2Word], playerStateDirtyBag2Mask) {
		writer.Patch(2, s.bag2.WriteDiff)
	}
}

func (s *BagState) WriteDiff(writer *Writer) {
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
}

func (s *Bag2State) WriteDiff(writer *Writer) {
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
}

func writeItemPatch(writer *Writer, item *testdata.Item, patch *itemPatch) {
	if HasDirty(patch.dirty[itemStateDirtyCountWord], itemStateDirtyCountMask) {
		writer.Int32(2, item.Count)
	}
}

func (s *PlayerState) ClearDirty() {
	ClearDirty(s.dirty[:])
	s.bag.ClearDirty()
	s.bag2.ClearDirty()
}

func (s *BagState) ClearDirty() {
	ClearDirty(s.dirty[:])
	s.itemChanges.ClearDirty()
}

func (s *Bag2State) ClearDirty() {
	ClearDirty(s.dirty[:])
	s.itemChanges.ClearDirty()
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
	items := state.LoadBag().Items()
	items2 := state.LoadBag2().Items()

	_, _ = items.Load(1002)
	_, _ = items2.Load(2002)
	if len(state.bag.itemChanges.changes) != 0 || len(state.bag2.itemChanges.changes) != 0 {
		t.Fatal("reading items must not create changes")
	}

	bagItem, _ := items.Load(1001)
	bagItem.SetCount(11)
	bagItem.SetCount(12)
	bag2Item, _ := items2.Load(2001)
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

	bagItem, _ = items.Load(1001)
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
	items := state.LoadBag().Items()
	items2 := state.LoadBag2().Items()

	item, ok := items.Load(1001)
	if !ok || item.LoadCount() != 10 {
		t.Fatal("load existing item failed")
	}
	item.SetCount(11)
	items.Store(1001, &testdata.Item{Id: 1001, Count: 12})

	items.Store(2001, &testdata.Item{Id: 2001, Count: 1})
	item, _ = items.Load(2001)
	item.SetCount(2)

	item, _ = items.Load(1002)
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

func applyTrackedPlayerDiff(player *testdata.Player, data []byte) error {
	reader := NewReader(data)
	for {
		fieldNumber, operation, payload, ok, err := reader.Next()
		if err != nil || !ok {
			return err
		}
		if operation != OperationPatch {
			return ErrInvalidData
		}
		switch fieldNumber {
		case 1:
			if err := applyBagDiff(player.Bag, payload); err != nil {
				return err
			}
		case 2:
			if err := applyBag2Diff(player.Bag2, payload); err != nil {
				return err
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
		if fieldNumber != 2 {
			return ErrInvalidData
		}
		valueReader := NewValueReader(payload)
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
	}
}

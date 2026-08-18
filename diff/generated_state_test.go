package diff_test

import (
	"testing"

	. "github.com/2comjie/nova/diff"
	"github.com/2comjie/nova/diff/testdata"
	"google.golang.org/protobuf/proto"
)

func TestGeneratedStateEndToEnd(t *testing.T) {
	original := &testdata.Player{
		Bag: &testdata.Bag{
			Capacity: 5,
			Items: map[uint64]*testdata.Item{
				1: {Id: 1, Count: 10},
				2: {Id: 2, Count: 20},
			},
			Order: []uint64{1, 2},
		},
		Bag2: &testdata.Bag2{Capacity: 8},
	}
	replica := proto.Clone(original).(*testdata.Player)
	state := testdata.NewPlayerState(original)
	bag, _ := state.LoadBag()

	bag.StoreCapacity(6)
	items := bag.Items()
	item, _ := items.Load(1)
	item.StoreCount(11)
	items.Store(3, &testdata.Item{Id: 3, Count: 30})
	items.Delete(2)

	order := bag.Order()
	order.Store(0, 10)
	order.Append(3)
	order.Insert(1, 20)
	order.Delete(2)
	order.Move(2, 0)

	slotValue := &testdata.Item{Id: 10, Count: 1}
	slots := bag.Slots()
	slots.Append(slotValue)
	slotValue.Count = 99
	slot, _ := slots.Load(0)
	slot.StoreCount(2)
	slots.Append(&testdata.Item{Id: 11, Count: 3})
	slots.Move(1, 0)

	blobValue := []byte{1, 2, 3}
	blobs := bag.Blobs()
	blobs.Append(blobValue)
	blobValue[0] = 9
	blobs.Insert(0, []byte{4, 5})
	blobs.Store(1, []byte{6, 7})

	counters := bag.Counters()
	counters.Store("gold", 100)
	counters.Store("silver", 200)
	counters.Delete("silver")

	writer := NewWriter(nil)
	state.WriteDiff(writer)
	if err := testdata.ApplyPlayerDiff(replica, writer.Data()); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(replica, original) {
		t.Fatalf("expected %v, got %v", original, replica)
	}
	state.ClearDirty()

	state.StoreBag(&testdata.Bag{Capacity: 100})
	bag, _ = state.LoadBag()
	bag.StoreCapacity(101)
	state.DeleteBag2()

	writer = NewWriter(nil)
	state.WriteDiff(writer)
	if err := testdata.ApplyPlayerDiff(replica, writer.Data()); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(replica, original) {
		t.Fatalf("expected %v, got %v", original, replica)
	}
}

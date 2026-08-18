package diff_test

import (
	"testing"

	. "github.com/2comjie/nova/diff"
	"github.com/2comjie/nova/diff/testdata/bagpkg"
	"github.com/2comjie/nova/diff/testdata/playerpkg"
	"google.golang.org/protobuf/proto"
)

func TestCrossPackageApplyHooks(t *testing.T) {
	source := &playerpkg.Player{
		Id: 1001,
		Bag: &bagpkg.Bag{Items: map[uint64]*bagpkg.Item{
			2001: {Id: 2001, Count: 10},
		}},
	}
	target := proto.Clone(source).(*playerpkg.Player)
	state := playerpkg.NewPlayerState(source)
	item, ok := state.GetBag().Items().GetValue(2001)
	if !ok {
		t.Fatal("item 2001 not found")
	}
	item.SetCount(7)

	writer := NewWriter(nil)
	state.WriteDiff(writer)
	events := make([]string, 0, 2)
	hooks := &playerpkg.PlayerApplyHooks{
		Bag: &bagpkg.BagApplyHooks{
			OnItemsCountChanged: func(key uint64, oldValue, newValue int32) {
				if key != 2001 || oldValue != 10 || newValue != 7 {
					t.Fatalf("unexpected item event: key=%d old=%d new=%d", key, oldValue, newValue)
				}
				events = append(events, "bag.item.count")
			},
		},
		OnBagPatch: func(oldValue, newValue *bagpkg.Bag) {
			if oldValue.Items[2001].Count != 10 || newValue.Items[2001].Count != 7 {
				t.Fatalf("unexpected bag event: old=%d new=%d", oldValue.Items[2001].Count, newValue.Items[2001].Count)
			}
			events = append(events, "player.bag")
		},
	}
	if err := playerpkg.ApplyPlayerDiffWithHooks(target, writer.Data(), hooks); err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0] != "bag.item.count" || events[1] != "player.bag" {
		t.Fatalf("unexpected event order: %v", events)
	}
	if !proto.Equal(target, source) {
		t.Fatalf("diff result does not match source\nsource: %v\nresult: %v", source, target)
	}
}

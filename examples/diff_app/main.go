package main

import (
	"fmt"

	"github.com/2comjie/nova/diff"
	"github.com/2comjie/nova/examples/diff_app/bag"
	"github.com/2comjie/nova/examples/diff_app/item"
	"github.com/2comjie/nova/examples/diff_app/player"
)

func init() {
	diff.ListenBefore(player.PlayerDiff.Level(), func(change *diff.Change[int32]) {
		if change.NewValue < 1 {
			change.Cancel("等级不能小于1")
		}
		if change.NewValue > 100 {
			change.Replace(100)
		}
	})

	diff.ListenBefore(player.PlayerDiff.Bag().Items().Any().Count(), func(change *diff.Change[int32]) {
		if change.NewValue < 0 {
			change.Cancel("道具数量不能小于0")
		}
	})

	diff.ListenMapBefore(player.PlayerDiff.Bag().Items().Changes(), func(change *diff.MapChange[uint64, *item.Item[int32]]) {
		if change.Operation == diff.ChangeMapStore && change.Key != change.NewValue.GetItemId() {
			change.Cancel("ItemId必须和Map key一致")
		}
	})

	diff.ListenBefore(item.ItemDiff[int64]().Count(), func(change *diff.Change[int64]) {
		if change.NewValue > 999 {
			change.Replace(999)
		}
	})
}

func main() {
	writer := diff.NewWriter()
	playerValue := &player.Player{}
	playerValue.InitLink(writer)

	bagValue := &bag.Bag{}
	playerValue.SetBag(bagValue)

	itemValue := &item.Item[int32]{}
	itemValue.SetItemId(1001)
	itemValue.SetCount(5)

	playerValue.SetUid(10001)
	playerValue.SetName("taoxi")
	playerValue.SetLevel(20)
	playerValue.GetBag().Items().Store(1001, itemValue)
	playerValue.GetBag().Order().Append(itemValue)
	playerValue.Scores().Store(1, 100)
	playerValue.RecentLevels().Append(20)

	full := playerValue.Snapshot()
	playerValue.Commit()

	playerValue.SetLevel(200)
	itemValue.SetCount(-1)
	itemValue.SetCount(12)
	playerValue.Scores().Store(1, 120)
	playerValue.RecentLevels().Append(200)
	delta := playerValue.Commit()

	replica := &player.Player{}
	if err := replica.LoadSnapshot(full); err != nil {
		panic(err)
	}
	if err := replica.Merge(delta); err != nil {
		panic(err)
	}

	replicaItem, _ := replica.GetBag().Items().Load(1001)
	score, _ := replica.Scores().Load(1)
	latestLevel := replica.RecentLevels().GetValue(replica.RecentLevels().Len() - 1)
	fmt.Printf("跨包合并: uid=%d level=%d itemCount=%d score=%d recentLevel=%d deltaBytes=%d\n",
		replica.GetUid(), replica.GetLevel(), replicaItem.GetCount(), score, latestLevel, len(delta))

	genericWriter := diff.NewWriter()
	genericItem := &item.Item[int64]{}
	genericItem.InitLink(genericWriter)
	genericItem.SetItemId(2001)
	genericItem.SetCount(1200)
	fmt.Printf("范型Item: id=%d count=%d deltaBytes=%d\n",
		genericItem.GetItemId(), genericItem.GetCount(), len(genericItem.Commit()))
}

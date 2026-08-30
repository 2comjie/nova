//go:build !diff_fast

package main

import (
	"fmt"

	"github.com/2comjie/nova/diff"
)

func init() {
	diff.ListenBefore(PlayerDiff.Level(), func(c *diff.Change[int32]) {
		if c.NewValue < 1 {
			c.Cancel("等级不能小于1")
		}
		if c.NewValue > 100 {
			c.Replace(100)
		}
	})

	diff.ListenBefore(PlayerDiff.Bag().Items().Any().Count(), func(c *diff.Change[int32]) {
		if c.NewValue < 0 {
			c.Cancel("道具数量不能小于0")
		}
	})

	diff.ListenMapBefore(PlayerDiff.Bag().Items().Changes(), func(m *diff.MapChange[uint64, *Item]) {
		if m.Operation == diff.ChangeMapStore && m.Key != m.NewValue.GetItemId() {
			m.Cancel("ItemId必须和Map key 一致")
		}
	})

	diff.ListenSliceBefore(PlayerDiff.Bag().Order().Changes(), func(change *diff.SliceChange[*Item]) {
		if change.HasNew && change.NewValue == nil {
			change.Cancel("Order不能添加nil Item")
		}
	})

	diff.ListenSliceBefore(PlayerDiff.RecentLevels(), func(change *diff.SliceChange[int32]) {
		if change.HasNew && change.NewValue > 100 {
			change.Replace(100)
		}
	})

	diff.ListenAfter(PlayerDiff.Level(), func(change diff.Change[int32]) {
		fmt.Printf("等级变化: %d -> %d\n", change.OldValue, change.NewValue)
	})
}

func main() {
	writer := diff.NewWriter()
	player := &Player{}
	player.InitLink(writer)

	bag := &Bag{}
	player.SetBag(bag)

	item := &Item{}
	item.SetItemId(1001)
	item.SetCount(5)

	player.SetUid(10001)
	player.SetName("taoxi")
	player.SetLevel(20)
	player.GetBag().Items().Store(1001, item)
	player.GetBag().Order().Append(item)
	player.Scores().Store(1, 100)
	player.RecentLevels().Append(20)

	full := player.Snapshot()
	player.Commit()

	player.SetLevel(200)
	item.SetCount(-1)
	item.SetCount(12)
	player.GetBag().Items().Store(2002, item)
	player.GetBag().Order().Append(nil)
	player.Scores().Store(1, 120)
	player.RecentLevels().Append(200)

	delta := player.Commit()
	replica := &Player{}
	if err := replica.LoadSnapshot(full); err != nil {
		panic(err)
	}
	if err := replica.Merge(delta); err != nil {
		panic(err)
	}

	replicaItem, _ := replica.GetBag().Items().Load(1001)
	score, _ := replica.Scores().Load(1)
	latestLevel := replica.RecentLevels().GetValue(replica.RecentLevels().Len() - 1)
	fmt.Printf("合并结果: uid=%d level=%d itemCount=%d score=%d recentLevel=%d deltaBytes=%d\n",
		replica.GetUid(), replica.GetLevel(), replicaItem.GetCount(), score, latestLevel, len(delta))
}

package main

import (
	"fmt"

	"github.com/2comjie/nova/diff"
	"github.com/2comjie/nova/examples/diff_app/bag"
	"github.com/2comjie/nova/examples/diff_app/item"
	"github.com/2comjie/nova/examples/diff_app/player"
	"google.golang.org/protobuf/proto"
)

func main() {
	writer := diff.NewWriter()
	playerValue := new(player.Player)
	playerValue.InitLink(writer)

	bagValue := new(bag.Bag)
	playerValue.SetBag(bagValue)

	itemValue := new(item.Item)
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
	update := playerValue.Commit()
	fmt.Println(update)

	replica := new(player.Player)
	replica.LoadSnapshot(full)
	replica.Merge(update)

	replicaItem, _ := replica.GetBag().Items().Load(1001)
	score, _ := replica.Scores().Load(1)
	latestLevel := replica.RecentLevels().GetValue(replica.RecentLevels().Len() - 1)
	fmt.Printf("跨包合并: uid=%d level=%d itemCount=%d score=%d recentLevel=%d updateBytes=%d\n",
		replica.GetUid(), replica.GetLevel(), replicaItem.GetCount(), score, latestLevel, proto.Size(update))
}

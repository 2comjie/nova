package diff_test

import (
	"testing"

	"github.com/2comjie/nova/diff"
	"github.com/2comjie/nova/diff/testdata"
	"google.golang.org/protobuf/proto"
)

func TestSnapManagerCommit(t *testing.T) {
	value := &testdata.Player{Bag: &testdata.Bag{Capacity: 10}}
	replica := proto.Clone(value).(*testdata.Player)
	state := testdata.NewPlayerState(value)
	manager := diff.NewSnapManager[*testdata.Player](state, 100, 5)

	state.GetBag().SetCapacity(20)
	if !manager.Commit() {
		t.Fatal("脏数据没有生成增量")
	}
	if manager.Version() != 101 {
		t.Fatalf("版本错误: %d", manager.Version())
	}
	if state.IsDirty() {
		t.Fatal("提交后没有清除脏标记")
	}

	fullVersion, fullData, deltas := manager.Get(100)
	if fullVersion != 0 || fullData != nil || len(deltas) != 1 {
		t.Fatalf("增量结果错误: fullVersion=%d full=%d deltas=%d", fullVersion, len(fullData), len(deltas))
	}
	if deltas[0].BaseVersion != 100 || deltas[0].Version != 101 {
		t.Fatalf("增量版本错误: %+v", deltas[0])
	}
	if err := testdata.ApplyPlayerDiff(replica, deltas[0].Data); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(replica, value) {
		t.Fatalf("应用增量后的数据不一致: replica=%v value=%v", replica, value)
	}

	if manager.Commit() {
		t.Fatal("无脏数据时不应生成增量")
	}
	if manager.Version() != 101 {
		t.Fatalf("空提交改变了版本: %d", manager.Version())
	}
}

func TestSnapManagerFallsBackToCurrentFull(t *testing.T) {
	value := &testdata.Player{Bag: &testdata.Bag{Capacity: 10}}
	state := testdata.NewPlayerState(value)
	manager := diff.NewSnapManager[*testdata.Player](state, 100, 2)

	for capacity := int32(11); capacity <= 13; capacity++ {
		state.GetBag().SetCapacity(capacity)
		manager.Commit()
	}

	_, _, deltas := manager.Get(101)
	if len(deltas) != 2 {
		t.Fatalf("保留的增量数量错误: %d", len(deltas))
	}

	fullVersion, fullData, deltas := manager.Get(100)
	if fullVersion != 103 || len(fullData) == 0 || len(deltas) != 0 {
		t.Fatalf("全量回退错误: fullVersion=%d full=%d deltas=%d", fullVersion, len(fullData), len(deltas))
	}
	replica := &testdata.Player{}
	if err := proto.Unmarshal(fullData, replica); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(replica, value) {
		t.Fatalf("全量数据不一致: replica=%v value=%v", replica, value)
	}
	if manager.Version() != 103 {
		t.Fatalf("生成全量改变了版本: %d", manager.Version())
	}
}

func TestSnapManagerReturnsCachedFullWithDiffs(t *testing.T) {
	value := &testdata.Player{Bag: &testdata.Bag{Capacity: 10}}
	state := testdata.NewPlayerState(value)
	manager := diff.NewSnapManager[*testdata.Player](state, 100, 5)
	manager.BuildFull()

	state.GetBag().SetCapacity(11)
	manager.Commit()
	state.GetBag().SetCapacity(12)
	manager.Commit()

	fullVersion, fullData, deltas := manager.Get(1)
	if fullVersion != 100 || len(fullData) == 0 || len(deltas) != 2 {
		t.Fatalf("缓存全量结果错误: fullVersion=%d full=%d deltas=%d", fullVersion, len(fullData), len(deltas))
	}

	replica := &testdata.Player{}
	if err := proto.Unmarshal(fullData, replica); err != nil {
		t.Fatal(err)
	}
	for _, delta := range deltas {
		if err := testdata.ApplyPlayerDiff(replica, delta.Data); err != nil {
			t.Fatal(err)
		}
	}
	if !proto.Equal(replica, value) {
		t.Fatalf("全量加增量后的数据不一致: replica=%v value=%v", replica, value)
	}
}

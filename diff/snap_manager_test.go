package diff_test

import (
	"testing"

	"github.com/2comjie/nova/diff"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type int64State struct {
	value *wrapperspb.Int64Value
	dirty bool
}

func (s *int64State) GetRawValue() *wrapperspb.Int64Value {
	return s.value
}

func (s *int64State) IsDirty() bool {
	return s.dirty
}

func (s *int64State) SetValue(value int64) {
	if s.value.Value == value {
		return
	}
	s.value.Value = value
	s.dirty = true
}

func (s *int64State) WriteDiff(writer *diff.Writer) {
	writer.Int64(1, s.value.Value)
}

func (s *int64State) ClearDirty() {
	s.dirty = false
}

func TestSnapManagerServerAPI(t *testing.T) {
	serverValue := wrapperspb.Int64(10)
	state := &int64State{value: serverValue}
	manager := diff.NewSnapManager[*wrapperspb.Int64Value](state, 100, 2)

	state.SetValue(20)
	delta, changed := manager.Commit()
	if !changed || delta.BaseVersion != 100 || delta.Version != 101 || manager.Version() != 101 {
		t.Fatalf("提交结果错误: changed=%t delta=%+v version=%d", changed, delta, manager.Version())
	}
	if _, changed := manager.Commit(); changed {
		t.Fatal("没有脏数据时生成了增量")
	}

	syncData, changed := manager.WriteSync(100, nil)
	if !changed {
		t.Fatal("没有生成纯增量同步包")
	}
	reader, err := diff.NewSyncReader(syncData)
	if err != nil {
		t.Fatal(err)
	}
	if reader.HasFull() || reader.BaseVersion() != 100 || reader.Version() != 101 {
		t.Fatalf("纯增量同步头错误: full=%t version=%d->%d", reader.HasFull(), reader.BaseVersion(), reader.Version())
	}
	delta, ok, err := reader.NextDiff()
	if err != nil || !ok {
		t.Fatalf("读取增量失败: ok=%t err=%v", ok, err)
	}
	patchReader := diff.NewReader(delta.Data)
	fieldNumber, operation, payload, ok, err := patchReader.Next()
	if err != nil || !ok || fieldNumber != 1 || operation != diff.OperationSetVarint || diff.DecodeInt64(payload) != 20 {
		t.Fatalf("增量数据错误: field=%d operation=%d value=%d ok=%t err=%v", fieldNumber, operation, diff.DecodeInt64(payload), ok, err)
	}
	if _, changed := manager.WriteSync(101, nil); changed {
		t.Fatal("版本相同时生成了同步包")
	}

	snapshotVersion, snapshotData := manager.BuildSnapshot()
	if snapshotVersion != 101 {
		t.Fatalf("快照版本错误: %d", snapshotVersion)
	}
	restoredValue := &wrapperspb.Int64Value{}
	if err := proto.Unmarshal(snapshotData, restoredValue); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(restoredValue, serverValue) {
		t.Fatalf("快照数据错误: restored=%v server=%v", restoredValue, serverValue)
	}

	restoredManager := diff.NewSnapManager[*wrapperspb.Int64Value](&int64State{value: restoredValue}, snapshotVersion, 2)
	syncData, changed = restoredManager.WriteSync(100, nil)
	if !changed {
		t.Fatal("重建后没有回退到全量同步")
	}
	reader, err = diff.NewSyncReader(syncData)
	if err != nil {
		t.Fatal(err)
	}
	if !reader.HasFull() || reader.BaseVersion() != snapshotVersion || reader.Version() != snapshotVersion {
		t.Fatalf("重建后的全量同步头错误: full=%t version=%d->%d", reader.HasFull(), reader.BaseVersion(), reader.Version())
	}
	fullValue := &wrapperspb.Int64Value{}
	if err := proto.Unmarshal(reader.FullData(), fullValue); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(fullValue, serverValue) {
		t.Fatalf("重建后的全量错误: full=%v server=%v", fullValue, serverValue)
	}
}

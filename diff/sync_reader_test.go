package diff

import (
	"bytes"
	"errors"
	"testing"
)

func TestSyncReaderDiff(t *testing.T) {
	writer := NewSyncWriter(nil)
	writer.WriteDiff([]Delta{
		{BaseVersion: 100, Version: 101, Data: []byte{1, 2}},
		{BaseVersion: 101, Version: 102, Data: []byte{3}},
	})

	reader, err := NewSyncReader(writer.Data())
	if err != nil {
		t.Fatal(err)
	}
	if reader.HasFull() || reader.BaseVersion() != 100 || reader.Version() != 102 {
		t.Fatalf("同步头错误: full=%t version=%d->%d", reader.HasFull(), reader.BaseVersion(), reader.Version())
	}

	for _, want := range [][]byte{{1, 2}, {3}} {
		delta, ok, err := reader.NextDiff()
		if err != nil || !ok {
			t.Fatalf("读取增量失败: ok=%t err=%v", ok, err)
		}
		if !bytes.Equal(delta.Data, want) {
			t.Fatalf("增量错误: %v", delta.Data)
		}
	}
	if _, ok, err := reader.NextDiff(); err != nil || ok {
		t.Fatalf("增量没有正确结束: ok=%t err=%v", ok, err)
	}
}

func TestSyncReaderFull(t *testing.T) {
	writer := NewSyncWriter(nil)
	writer.WriteFull(100, nil, []Delta{
		{BaseVersion: 100, Version: 101, Data: []byte{4, 5}},
	})

	reader, err := NewSyncReader(writer.Data())
	if err != nil {
		t.Fatal(err)
	}
	if !reader.HasFull() || reader.BaseVersion() != 100 || reader.Version() != 101 || len(reader.FullData()) != 0 {
		t.Fatalf("全量头错误: full=%t version=%d->%d data=%v", reader.HasFull(), reader.BaseVersion(), reader.Version(), reader.FullData())
	}
	delta, ok, err := reader.NextDiff()
	if err != nil || !ok || !bytes.Equal(delta.Data, []byte{4, 5}) {
		t.Fatalf("读取增量失败: delta=%+v ok=%t err=%v", delta, ok, err)
	}
}

func TestSyncReaderRejectsInvalidData(t *testing.T) {
	tests := [][]byte{
		nil,
		{2, 0, 0},
		{0, 2, 1},
		{SyncFlagFull, 0, 0},
		{0, 0, 1},
		{0, 0, 1, 2, 1},
		{0, 0, 0, 1, 1},
	}

	for _, data := range tests {
		reader, err := NewSyncReader(data)
		if err == nil {
			for {
				_, ok, readErr := reader.NextDiff()
				if readErr != nil {
					err = readErr
					break
				}
				if !ok {
					break
				}
			}
		}
		if !errors.Is(err, ErrInvalidData) {
			t.Fatalf("没有拒绝非法数据 %v: %v", data, err)
		}
	}
}

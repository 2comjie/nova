package diff

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestSyncWriterDiff(t *testing.T) {
	writer := NewSyncWriter(nil)
	writer.WriteDiff([]Delta{
		{BaseVersion: 100, Version: 101, Data: []byte{1, 2}},
		{BaseVersion: 101, Version: 102, Data: []byte{3}},
	})

	data := writer.Data()
	if data[0] != 0 {
		t.Fatalf("flags错误: %d", data[0])
	}
	data = data[1:]
	baseVersion, size := binary.Uvarint(data)
	data = data[size:]
	version, size := binary.Uvarint(data)
	data = data[size:]
	if baseVersion != 100 || version != 102 {
		t.Fatalf("版本错误: %d -> %d", baseVersion, version)
	}

	for _, want := range [][]byte{{1, 2}, {3}} {
		length, size := binary.Uvarint(data)
		data = data[size:]
		if !bytes.Equal(data[:length], want) {
			t.Fatalf("增量错误: %v", data[:length])
		}
		data = data[length:]
	}
	if len(data) != 0 {
		t.Fatalf("存在多余数据: %v", data)
	}
}

func TestSyncWriterFull(t *testing.T) {
	writer := NewSyncWriter(nil)
	writer.WriteFull(100, nil, []Delta{
		{BaseVersion: 100, Version: 101, Data: []byte{4, 5}},
	})

	data := writer.Data()
	if data[0] != SyncFlagFull {
		t.Fatalf("flags错误: %d", data[0])
	}
	data = data[1:]
	fullVersion, size := binary.Uvarint(data)
	data = data[size:]
	version, size := binary.Uvarint(data)
	data = data[size:]
	fullLength, size := binary.Uvarint(data)
	data = data[size+int(fullLength):]
	if fullVersion != 100 || version != 101 || fullLength != 0 {
		t.Fatalf("全量头错误: %d -> %d length=%d", fullVersion, version, fullLength)
	}
	diffLength, size := binary.Uvarint(data)
	data = data[size:]
	if !bytes.Equal(data[:diffLength], []byte{4, 5}) {
		t.Fatalf("增量错误: %v", data[:diffLength])
	}
}

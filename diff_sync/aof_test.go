package diff_sync

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestStoreAppend(t *testing.T) {
	dir := t.TempDir()

	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("hello")
	store.append(1, 2, false, payload)
	store.Shutdown()

	value, err := openAOF(dir, aofSegmentSize)
	if err != nil {
		t.Fatal(err)
	}
	defer value.Close()

	if value.offset != aofHeaderSize+len(payload) {
		t.Fatalf("offset = %d", value.offset)
	}

	if binary.LittleEndian.Uint64(value.data[8:16]) != 1 {
		t.Fatal("Id 写入错误")
	}

	if binary.LittleEndian.Uint64(value.data[16:24]) != 2 {
		t.Fatal("Version 写入错误")
	}

	if !bytes.Equal(value.data[aofHeaderSize:value.offset], payload) {
		t.Fatal("Payload 写入错误")
	}
}

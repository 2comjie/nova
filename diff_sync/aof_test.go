package diff_sync

import (
	"bytes"
	"encoding/binary"
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

func TestStoreAppend(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte("hello")
	store.append(1, 10, 2, payload)
	store.Shutdown()

	value, err := openAOF(filepath.Join(dir, aofName), aofFileSize)
	if err != nil {
		t.Fatal(err)
	}
	defer value.Close()

	if value.offset != aofHeaderSize+len(payload) {
		t.Fatalf("offset = %d", value.offset)
	}
	if binary.LittleEndian.Uint64(value.data[8:16]) != 1 {
		t.Fatal("TypeId 写入错误")
	}
	if binary.LittleEndian.Uint64(value.data[16:24]) != 10 {
		t.Fatal("DataId 写入错误")
	}
	if binary.LittleEndian.Uint64(value.data[24:32]) != 2 {
		t.Fatal("Version 写入错误")
	}
	if !bytes.Equal(value.data[aofHeaderSize:value.offset], payload) {
		t.Fatal("Payload 写入错误")
	}
}

func TestAOFReplay(t *testing.T) {
	value, err := openAOF(filepath.Join(t.TempDir(), aofName), 128)
	if err != nil {
		t.Fatal(err)
	}

	if err := value.Append(1, 10, 1, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := value.Append(2, 20, 2, []byte("second")); err != nil {
		t.Fatal(err)
	}

	var records []record
	if err := value.Replay(func(value record) error {
		records = append(records, value)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if len(records) != 2 {
		t.Fatalf("record count = %d", len(records))
	}
	if records[0].TypeId != 1 || records[0].DataId != 10 || records[0].Version != 1 || string(records[0].Payload) != "first" {
		t.Fatalf("first record = %+v", records[0])
	}
	if records[1].TypeId != 2 || records[1].DataId != 20 || records[1].Version != 2 || string(records[1].Payload) != "second" {
		t.Fatalf("second record = %+v", records[1])
	}

	if err := value.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRewrite(t *testing.T) {
	dir := t.TempDir()
	value, err := openAOF(filepath.Join(dir, aofName), aofFileSize)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Append(1, 10, 1, []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := value.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	flushed := make(chan struct{})
	store.rewriteAOF(func() error {
		store.append(1, 10, 2, []byte("new"))
		close(flushed)
		return nil
	})
	<-flushed
	store.Shutdown()

	value, err = openAOF(filepath.Join(dir, aofName), aofFileSize)
	if err != nil {
		t.Fatal(err)
	}
	defer value.Close()

	var records []record
	if err := value.Replay(func(value record) error {
		records = append(records, value)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Version != 2 || string(records[0].Payload) != "new" {
		t.Fatalf("records = %+v", records)
	}
}

func TestStoreRewriteFailure(t *testing.T) {
	dir := t.TempDir()
	value, err := openAOF(filepath.Join(dir, aofName), aofFileSize)
	if err != nil {
		t.Fatal(err)
	}
	if err := value.Append(1, 10, 1, []byte("old")); err != nil {
		t.Fatal(err)
	}
	if err := value.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	flushErr := errors.New("flush failed")
	flushed := make(chan struct{})
	store.rewriteAOF(func() error {
		store.append(1, 10, 2, []byte("new"))
		close(flushed)
		return flushErr
	})
	<-flushed
	store.Shutdown()

	value, err = openAOF(filepath.Join(dir, aofName), aofFileSize)
	if err != nil {
		t.Fatal(err)
	}
	defer value.Close()

	var records []record
	if err := value.Replay(func(value record) error {
		records = append(records, value)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Version != 1 || records[1].Version != 2 {
		t.Fatalf("records = %+v", records)
	}
}

func TestStoreRewriteWhileWriting(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	flushStarted := make(chan struct{})
	flushFinished := make(chan struct{})
	store.rewriteAOF(func() error {
		close(flushStarted)
		<-flushFinished
		return nil
	})
	<-flushStarted

	var wait sync.WaitGroup
	for index := uint64(1); index <= 100; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			store.append(1, index, 1, []byte("value"))
		}()
	}
	wait.Wait()
	close(flushFinished)
	store.Shutdown()

	value, err := openAOF(filepath.Join(dir, aofName), aofFileSize)
	if err != nil {
		t.Fatal(err)
	}
	defer value.Close()

	count := 0
	if err := value.Replay(func(record) error {
		count++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != 100 {
		t.Fatalf("record count = %d", count)
	}
}

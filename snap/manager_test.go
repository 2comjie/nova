package snap

import (
	"bytes"
	"testing"
)

func TestManagerPull(t *testing.T) {
	manager := NewManager(100, 3, func() []byte {
		return []byte("full-104")
	})

	manager.Bind(1, 100)
	manager.Append([]byte("101"))
	manager.Append([]byte("102"))

	result := manager.Pull(1)
	if result.Full || result.BaseVersion != 100 || result.Version != 102 || len(result.Deltas) != 2 {
		t.Fatalf("result = %+v", result)
	}
	if !bytes.Equal(result.Deltas[0], []byte("101")) || !bytes.Equal(result.Deltas[1], []byte("102")) {
		t.Fatalf("deltas = %q", result.Deltas)
	}

	if !manager.Ack(1, 102) {
		t.Fatal("ack failed")
	}
	result = manager.Pull(1)
	if result.Full || result.BaseVersion != 102 || result.Version != 102 || len(result.Deltas) != 0 {
		t.Fatalf("acked result = %+v", result)
	}
}

func TestManagerIgnoresEmptyDelta(t *testing.T) {
	manager := NewManager(100, 3, func() []byte { return nil })
	if version := manager.Append([]byte{0}); version != 100 {
		t.Fatalf("version = %d", version)
	}
}

func TestManagerFullSnapshot(t *testing.T) {
	manager := NewManager(100, 3, func() []byte {
		return []byte("full-104")
	})
	manager.Append([]byte("101"))
	manager.Append([]byte("102"))
	manager.Append([]byte("103"))
	manager.Append([]byte("104"))

	manager.Bind(1, 100)
	result := manager.Pull(1)
	if !result.Full || result.Version != 104 || !bytes.Equal(result.Snapshot, []byte("full-104")) {
		t.Fatalf("old client result = %+v", result)
	}

	manager.Bind(2, 105)
	result = manager.Pull(2)
	if !result.Full || result.Version != 104 {
		t.Fatalf("ahead client result = %+v", result)
	}
}

func TestManagerClientVersions(t *testing.T) {
	manager := NewManager(10, 2, func() []byte { return []byte("full") })
	manager.Bind(1, 10)
	manager.Bind(2, 9)
	manager.Append([]byte("11"))

	if !manager.Ack(1, 11) {
		t.Fatal("ack failed")
	}
	if manager.Ack(2, 12) {
		t.Fatal("accepted future version")
	}

	version, exists := manager.ClientVersion(1)
	if !exists || version != 11 {
		t.Fatalf("client 1 version = %d %v", version, exists)
	}
	version, exists = manager.ClientVersion(2)
	if !exists || version != 9 {
		t.Fatalf("client 2 version = %d %v", version, exists)
	}

	manager.Unbind(1)
	if _, exists := manager.ClientVersion(1); exists {
		t.Fatal("client 1 still bound")
	}
}

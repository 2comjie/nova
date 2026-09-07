package snap

import (
	"testing"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestClientVersions(t *testing.T) {
	value := int32(10)
	manager := NewManager[uint64](uint64(0), 2, func() *wrapperspb.Int32Value {
		return wrapperspb.Int32(value)
	})
	manager.Bind(1)
	if manager.Ack(1, 0) || manager.Ack(2, 0) {
		t.Fatal("ack accepted before sending or binding")
	}
	first := manager.Pull(1)
	if first.Full.GetValue() != 10 || first.Version != 0 || len(first.Updates) != 0 {
		t.Fatal("new subscriber did not receive full state")
	}
	if _, ready := manager.ClientVersion(1); ready {
		t.Fatal("unacknowledged baseline marked ready")
	}
	if !manager.Ack(1, first.Version) {
		t.Fatal("initial baseline ack rejected")
	}
	manager.Resume(2, 0)
	value = 11
	manager.Append(wrapperspb.Int32(value))
	value = 12
	manager.Append(wrapperspb.Int32(value))
	result := manager.Pull(1)
	if result.Full != nil || result.BaseVersion != 0 || result.Version != 2 ||
		len(result.Updates) != 2 || result.Updates[0].Value != 11 || result.Updates[1].Value != 12 {
		t.Fatal("incremental history mismatch", result)
	}
	if !manager.Ack(1, 2) || manager.Ack(1, 1) {
		t.Fatal("ack did not advance monotonically")
	}
	value = 13
	manager.Append(wrapperspb.Int32(value))
	if manager.Ack(1, 3) {
		t.Fatal("accepted version not sent to this client")
	}
	result = manager.Pull(2)
	if result.Full.GetValue() != 13 || result.Version != 3 || len(result.Updates) != 0 {
		t.Fatal("expired history did not fall back to full state")
	}
	if manager.Ack(2, 2) || !manager.Ack(2, 3) {
		t.Fatal("full baseline ack mismatch")
	}
	result = manager.Pull(1)
	if result.BaseVersion != 2 || len(result.Updates) != 1 || result.Updates[0].Value != 13 {
		t.Fatal("one client affected another client's progress")
	}
	manager.Unbind(1)
	if manager.Ack(1, 3) {
		t.Fatal("late ack recreated an unbound client")
	}
}

func TestDelayedFullAck(t *testing.T) {
	manager := NewManager[uint64](uint64(5), 2, func() *wrapperspb.Int32Value {
		return wrapperspb.Int32(1)
	})
	manager.Bind(1)
	first := manager.Pull(1)
	manager.Append(wrapperspb.Int32(2))
	manager.Pull(1)
	if manager.Ack(1, 4) || !manager.Ack(1, first.Version) {
		t.Fatal("repeated full sends prevented acknowledgment of an earlier full state")
	}
	result := manager.Pull(1)
	if result.BaseVersion != 5 || len(result.Updates) != 1 || result.Updates[0].Value != 2 {
		t.Fatal("failed to continue from acknowledged full state")
	}
	manager.Resume(2, 100)
	result = manager.Pull(2)
	if result.Full == nil || !manager.Ack(2, result.Version) {
		t.Fatal("future client version did not reset to a valid full baseline")
	}
}

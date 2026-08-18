package diff

import "testing"

func TestDirtyBits(t *testing.T) {
	bits := make([]uint64, 10)
	indexes := []uint32{0, 63, 64, 127, 128, 599}

	for _, index := range indexes {
		if !MarkDirty(bits, index) {
			t.Fatalf("index %d should become dirty", index)
		}
		if !HasDirty(bits, index) {
			t.Fatalf("index %d should be dirty", index)
		}
		if MarkDirty(bits, index) {
			t.Fatalf("index %d should not become dirty twice", index)
		}
	}

	if !AnyDirty(bits) {
		t.Fatal("expected dirty bits")
	}

	ClearDirty(bits)
	if AnyDirty(bits) {
		t.Fatal("expected clean bits")
	}
	for _, index := range indexes {
		if HasDirty(bits, index) {
			t.Fatalf("index %d should be clean", index)
		}
	}

	if !MarkDirty(bits, 599) {
		t.Fatal("index 599 should become dirty after clear")
	}
}

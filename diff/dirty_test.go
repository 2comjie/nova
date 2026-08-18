package diff

import "testing"

func TestDirtyBits(t *testing.T) {
	bits := make([]uint64, 10)
	positions := []struct {
		word uint32
		mask uint64
	}{
		{word: 0, mask: 1},
		{word: 0, mask: uint64(1) << 63},
		{word: 1, mask: 1},
		{word: 1, mask: uint64(1) << 63},
		{word: 2, mask: 1},
		{word: 9, mask: uint64(1) << 23},
	}

	for _, position := range positions {
		if !MarkDirty(&bits[position.word], position.mask) {
			t.Fatalf("word %d mask %d should become dirty", position.word, position.mask)
		}
		if !HasDirty(bits[position.word], position.mask) {
			t.Fatalf("word %d mask %d should be dirty", position.word, position.mask)
		}
		if MarkDirty(&bits[position.word], position.mask) {
			t.Fatalf("word %d mask %d should not become dirty twice", position.word, position.mask)
		}
	}

	if !AnyDirty(bits) {
		t.Fatal("expected dirty bits")
	}

	ClearDirty(bits)
	if AnyDirty(bits) {
		t.Fatal("expected clean bits")
	}
	for _, position := range positions {
		if HasDirty(bits[position.word], position.mask) {
			t.Fatalf("word %d mask %d should be clean", position.word, position.mask)
		}
	}

	if !MarkDirty(&bits[9], uint64(1)<<23) {
		t.Fatal("index 599 should become dirty after clear")
	}
}

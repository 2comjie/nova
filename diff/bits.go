package diff

type bitWords []uint64

type Bits struct {
	first    uint64
	overflow *bitWords
}

func (b *Bits) Init(size uint32) {
	b.first = 0
	if size <= 64 {
		b.overflow = nil
		return
	}

	words := make(bitWords, (size-1)>>6)
	b.overflow = &words
}

func (b *Bits) Mark(diffIndex uint32) (marked bool, firstDirty bool) {
	word := diffIndex >> 6
	mask := uint64(1) << (diffIndex & 63)

	target := &b.first

	if word != 0 {
		target = &(*b.overflow)[word-1]
	}

	if *target&mask != 0 {
		return false, false
	}

	firstDirty = b.first == 0
	if firstDirty && b.overflow != nil {
		for _, value := range *b.overflow {
			if value != 0 {
				firstDirty = false
				break
			}
		}
	}

	*target |= mask
	return true, firstDirty
}

func (b *Bits) IsDirty(diffIndex uint32) bool {
	word := diffIndex >> 6
	mask := uint64(1) << (diffIndex & 63)

	if word == 0 {
		return b.first&mask != 0
	}
	return (*b.overflow)[word-1]&mask != 0
}

func (b *Bits) Clear() {
	b.first = 0
	if b.overflow != nil {
		clear(*b.overflow)
	}
}

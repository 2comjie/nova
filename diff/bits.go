package diff

type bitWords []uint64

type Bits struct {
	first    uint64
	overflow *bitWords
}

func (b *Bits) Init(fieldCount uint32) {
	b.first = 0
	if fieldCount <= 64 {
		b.overflow = nil
		return
	}

	words := make(bitWords, (fieldCount-1)>>6)
	b.overflow = &words
}

func (b *Bits) Mark(word uint32, mask uint64) (marked bool, firstDirty bool) {
	if b.Has(word, mask) {
		return false, false
	}

	firstDirty = !b.Any()
	if word == 0 {
		b.first |= mask
		return true, firstDirty
	}
	(*b.overflow)[word-1] |= mask
	return true, firstDirty
}

func (b *Bits) Has(word uint32, mask uint64) bool {
	if word == 0 {
		return b.first&mask != 0
	}
	return (*b.overflow)[word-1]&mask != 0
}

func (b *Bits) Any() bool {
	if b.first != 0 {
		return true
	}
	if b.overflow == nil {
		return false
	}
	for _, word := range *b.overflow {
		if word != 0 {
			return true
		}
	}
	return false
}

func (b *Bits) Clear() {
	b.first = 0
	if b.overflow != nil {
		clear(*b.overflow)
	}
}

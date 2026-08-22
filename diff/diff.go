package diff

type DirtyObject struct {
	parents []ParentLink
	root    *DirtyObject
	bits    Bits
}

type Parent interface {
	DiffObject() *DirtyObject
	MarkDiffChildDirty(slot uint32)
}

type ParentLink struct {
	parent Parent
	slot   uint32
}

func (o *DirtyObject) InitBits(filedCount uint32) {
	o.bits.Init(filedCount)
}

func (o *DirtyObject) Mark(word uint32, mask uint64) {
	_, firstDirty := o.bits.Mark(word, mask)
	if !firstDirty {
		return
	}

	for _, link := range o.parents {
		link.parent.MarkDiffChildDirty(link.slot)
	}
}

func (o *DirtyObject) Has(word uint32, mask uint64) bool {
	return o.bits.Has(word, mask)
}

func (o *DirtyObject) IsDirty() bool {
	return o.bits.Any()
}

func (o *DirtyObject) ClearDirty() {
	o.bits.Clear()
}

package diff

type DirtyObject struct {
	parents []ParentLink
	bits    Bits
}

type Parent interface {
	DiffObject() *DirtyObject
	MarkDiffChildDirty(linkId uint32)
}

type ParentLink struct {
	parent Parent
	linkId uint32
}

func (o *DirtyObject) Init(fieldCount uint32) {
	o.parents = nil
	o.bits.Init(fieldCount)
}

func (o *DirtyObject) AddParent(parent Parent, linkId uint32) {
	parentObject := parent.DiffObject()
	for _, link := range o.parents {
		if link.parent.DiffObject() == parentObject && link.linkId == linkId {
			return
		}
	}
	o.parents = append(o.parents, ParentLink{parent: parent, linkId: linkId})
}

func (o *DirtyObject) RemoveParent(parent Parent, linkId uint32) {
	parentObject := parent.DiffObject()
	for index, link := range o.parents {
		if link.parent.DiffObject() != parentObject || link.linkId != linkId {
			continue
		}

		lastIndex := len(o.parents) - 1
		o.parents[index] = o.parents[lastIndex]
		o.parents[lastIndex] = ParentLink{}
		o.parents = o.parents[:lastIndex]
		return
	}
}

func (o *DirtyObject) Mark(word uint32, mask uint64) {
	_, firstDirty := o.bits.Mark(word, mask)
	if !firstDirty {
		return
	}

	for _, link := range o.parents {
		link.parent.MarkDiffChildDirty(link.linkId)
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

package diff

type Object struct {
	_    noCopy
	bits Bits // 这个 object 的所有字段的脏标记 16+n字节

	parent   ParentLink    // 8字节
	overflow *[]ParentLink // 8字节
}

type Parent interface {
	MarkChildDirty(key any) // 在父对象中标记自己脏的
}

type ParentLink struct {
	parent Parent
	key    any // 如果父节点是struct key就是 diff index 如果父对象是slice key就是slice index 如果父对象是map 这个key就是map key
}

func (o *Object) Init(diffCount uint32) {
	o.bits.Init(diffCount)
	o.parent = ParentLink{}
	o.overflow = nil
}

func (o *Object) AddParent(parent Parent, key any) {
	link := ParentLink{
		parent: parent,
		key:    key,
	}

	// 优先放在第一块空间
	if o.parent.parent == nil {
		o.parent = link
		return
	}

	if o.overflow == nil {
		parents := make([]ParentLink, 0, 1)
		o.overflow = &parents
	}
	*o.overflow = append(*o.overflow, link)
}

func (o *Object) RemoveParent(parent Parent, key any) {
	// 在第一块内存
	if o.parent.parent == parent && o.parent.key == key {
		// overflow 是空的 直接返回就行
		if o.overflow == nil || len(*o.overflow) == 0 {
			o.parent = ParentLink{}
			return
		}

		// overflow 有元素 原地向前移动
		links := *o.overflow
		last := len(links) - 1

		o.parent = links[last]
		links[last] = ParentLink{}
		links = links[:last]

		if len(links) == 0 {
			o.overflow = nil
		} else {
			*o.overflow = links
		}
		return
	}

	// 在第二块内存
	if o.overflow == nil {
		return
	}

	links := *o.overflow
	for index, link := range links {
		if link.parent != parent || link.key != key {
			continue
		}

		// 直接把最后一个元素交换过来 然后删除最后一个元素 原地删除
		last := len(links) - 1
		links[index] = links[last]
		links[last] = ParentLink{}
		links = links[:last]

		if len(links) == 0 {
			o.overflow = nil
		} else {
			*o.overflow = links
		}
		return
	}
}

func (o *Object) MarkDirty(diffIndex uint32) {
	// 标记脏
	_, firstDirty := o.bits.Mark(diffIndex)
	if !firstDirty {
		// 不是首次标脏 就不用再往父节点冒泡了
		return
	}

	// 父节点冒泡
	if o.parent.parent != nil {
		o.parent.parent.MarkChildDirty(o.parent.key)
	}

	if o.overflow == nil {
		return
	}

	for _, link := range *o.overflow {
		link.parent.MarkChildDirty(link.key)
	}
}

func (o *Object) IsDirty(diffIndex uint32) bool {
	return o.bits.IsDirty(diffIndex)
}

func (o *Object) ClearDirty() {
	o.bits.Clear()
}

func (o *Object) MarkChildDirty(key any) {
	// 直接标记某个字段是脏的即可
	o.MarkDirty(key.(uint32))
}

type noCopy struct {
}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

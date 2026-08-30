package diff

type Object struct {
	_ noCopy

	writer   *Writer
	parent   ParentLink
	overflow *[]ParentLink
}

type Parent interface {
	writeChildPatch(key any, childPath Path, operation Operation, value any)
	dispatchChildChange(key any, childPath listenerPath, phase listenerPhase, event *changeEvent)
}

type ParentLink struct {
	parent Parent
	key    any // 如果父节点是struct key就是 diff index 如果父对象是slice key就是slice index 如果父对象是map 这个key就是map key
}

func (o *Object) Init(writer *Writer) {
	o.writer = writer
	o.parent = ParentLink{}
	o.overflow = nil
}

func (o *Object) Commit() []byte {
	data := o.writer.Commit()
	o.writer.Reset()
	return data
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

func (o *Object) writeChildPatch(key any, childPath Path, operation Operation, value any) {
	path := make(Path, len(childPath)+1)
	path[0] = PathNode{KeyType: PathField, FieldIndex: key.(uint32)}
	copy(path[1:], childPath)
	o.writePatch(path, operation, value)
}

func (o *Object) dispatchChildChange(key any, childPath listenerPath, phase listenerPhase, event *changeEvent) {
	o.dispatchChange(prependListenerPath(listenerPathNode{
		selector:   listenerField,
		fieldIndex: key.(uint32),
	}, childPath), phase, event)
}

func (o *Object) dispatchChange(path listenerPath, phase listenerPhase, event *changeEvent) {
	if o.writer != nil {
		o.writer.dispatchChange(phase, path, event)
		return
	}

	if o.parent.parent != nil {
		o.parent.parent.dispatchChildChange(o.parent.key, path, phase, event)
	}
	if event.canceled || o.overflow == nil {
		return
	}

	for _, link := range *o.overflow {
		link.parent.dispatchChildChange(link.key, path, phase, event)
		if event.canceled {
			return
		}
	}
}

func (o *Object) writePatch(path Path, operation Operation, value any) {
	if o.writer != nil {
		o.writer.WritePatch(Patch{
			Path:      path,
			Operation: operation,
			Value:     value,
		})

		// root 节点才会有writer
		return
	}

	if o.parent.parent != nil {
		// 向上冒泡 path node 会填充完整
		o.parent.parent.writeChildPatch(o.parent.key, path, operation, value)
	}

	if o.overflow == nil {
		return
	}

	for _, link := range *o.overflow {
		link.parent.writeChildPatch(link.key, path, operation, value)
	}
}

type noCopy struct {
}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

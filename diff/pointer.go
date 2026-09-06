package diff

type PointerValue interface {
	comparable
	InitLink(writer *Writer)
	AddParent(parent Parent, key any)
	RemoveParent(parent Parent, key any)
}

type Pointer[T PointerValue] struct {
	_ noCopy

	value     T
	parent    *Object
	diffIndex uint32
}

func (p *Pointer[T]) Init(parent *Object, diffIndex uint32) {
	p.parent = parent
	p.diffIndex = diffIndex
}

func (p *Pointer[T]) GetValue() T {
	return p.value
}

func (p *Pointer[T]) SetValue(value T) bool {
	if p.value == value {
		return false
	}

	oldValue := p.value
	var zero T
	if oldValue != zero {
		oldValue.RemoveParent(p.parent, p.diffIndex)
	}

	p.value = value

	if value != zero {
		value.InitLink(nil)
		value.AddParent(p.parent, p.diffIndex)
		p.parent.writeChildPatch(p.diffIndex, nil, PointerSet, value)
	} else {
		p.parent.writeChildPatch(p.diffIndex, nil, PointerClear, nil)
	}
	return true
}

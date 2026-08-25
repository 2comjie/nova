package diff

type PointerValue interface {
	comparable
	AddParent(parent Parent, key any)
	RemoveParent(parent Parent, key any)
}

type Pointer[T PointerValue] struct {
	_ noCopy

	value     T
	parent    *Object
	diffIndex uint32
	replaced  bool
}

func (p *Pointer[T]) Init(parent *Object, diffIndex uint32) {
	p.parent = parent
	p.diffIndex = diffIndex
	var zero T
	if p.value != zero {
		p.value.AddParent(parent, diffIndex)
	}
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
	p.replaced = true

	if value != zero {
		value.AddParent(p.parent, p.diffIndex)
	}

	p.parent.MarkDirty(p.diffIndex)
	return true
}

func (p *Pointer[T]) IsReplaced() bool {
	return p.replaced
}

func (p *Pointer[T]) ClearDirty() {
	p.replaced = false
}

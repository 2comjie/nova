package diff

type PointerValue interface {
	comparable
	EnsureDiffLink()
	AddParent(parent Parent, key any)
	RemoveParent(parent Parent, key any)
	AppendDiffValue(data []byte) []byte
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
	var zero T
	if p.value != zero {
		p.value.AddParent(parent, diffIndex)
	}
}

func (p *Pointer[T]) Initialized() bool {
	return p.parent != nil
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
	operation := ChangeSet
	if value == zero {
		operation = ChangeClear
	}
	value, event, accepted := beforeValueChange(p.parent, p.diffIndex, operation, oldValue, value)
	if !accepted || oldValue == value {
		return false
	}
	if event != nil {
		if value == zero {
			event.operation = ChangeClear
		} else {
			event.operation = ChangeSet
		}
	}

	if oldValue != zero {
		oldValue.RemoveParent(p.parent, p.diffIndex)
	}

	p.value = value

	if value != zero {
		value.EnsureDiffLink()
		value.AddParent(p.parent, p.diffIndex)
		p.parent.writeChildPatch(p.diffIndex, nil, PointerSet, value)
	} else {
		p.parent.writeChildPatch(p.diffIndex, nil, PointerClear, nil)
	}
	afterValueChange(p.parent, p.diffIndex, event)
	return true
}

func (p *Pointer[T]) AppendValue(data []byte, diffIndex uint32) []byte {
	var zero T
	if p.value == zero {
		return data
	}

	data, lengthIndex := beginField(data, diffIndex)
	data = p.value.AppendDiffValue(data)
	return endValue(data, lengthIndex)
}

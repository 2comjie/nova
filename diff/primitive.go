package diff

import "github.com/2comjie/nova/generic"

type Primitive[T generic.Primitive] struct {
	_ noCopy

	parent    *Object
	diffIndex uint32
	value     T
}

func (p *Primitive[T]) Init(parent *Object, diffIndex uint32) {
	p.parent = parent
	p.diffIndex = diffIndex
}

func (p *Primitive[T]) GetValue() T {
	return p.value
}

func (p *Primitive[T]) SetValue(value T) bool {
	if p.value == value {
		return false
	}

	p.value = value
	p.parent.writeChildPatch(p.diffIndex, nil, PrimitiveSet, value)
	return true
}

func (p *Primitive[T]) AppendValue(data []byte, diffIndex uint32) []byte {
	var zero T
	if p.value == zero {
		return data
	}

	data, lengthIndex := beginField(data, diffIndex)
	data = appendPrimitive(data, p.value)
	return endValue(data, lengthIndex)
}

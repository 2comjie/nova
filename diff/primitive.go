package diff

import (
	"github.com/2comjie/nova/generic"
	"time"
)

func primitiveEqual[T generic.Primitive](left, right T) bool {
	if value, ok := any(left).(time.Time); ok {
		return value.UnixMilli() == any(right).(time.Time).UnixMilli()
	}
	return left == right
}

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

// LoadSnapshot 加载基线值，不记录增量。
func (p *Primitive[T]) LoadSnapshot(value T) {
	p.value = value
}

func (p *Primitive[T]) SetValue(value T) bool {
	if primitiveEqual(p.value, value) {
		return false
	}
	p.value = value
	p.parent.writeChildPatch(p.diffIndex, nil, PrimitiveSet, value)
	return true
}

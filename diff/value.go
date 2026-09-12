package diff

import (
	"reflect"
	"time"
)

type ObjectValue interface {
	InitLink(writer *Writer)
	AddParent(parent Parent, key any)
	RemoveParent(parent Parent, key any)
}

func objectValue(value any) ObjectValue {
	object, ok := value.(ObjectValue)
	if !ok || reflect.ValueOf(object).IsNil() {
		return nil
	}
	return object
}

func equalValue[T any](left, right T) bool {
	if value, ok := any(left).(time.Time); ok {
		other, ok := any(right).(time.Time)
		return ok && value.UnixMilli() == other.UnixMilli()
	}
	if any(left) == nil {
		return any(right) == nil
	}
	return reflect.ValueOf(left).Comparable() && any(left) == any(right)
}

type Value[T any] struct {
	_ noCopy

	parent    *Object
	diffIndex uint32
	value     T
}

func (v *Value[T]) Init(parent *Object, diffIndex uint32) {
	v.parent = parent
	v.diffIndex = diffIndex
}

func (v *Value[T]) GetValue() T {
	return v.value
}

func (v *Value[T]) LoadSnapshot(value T) {
	if old := objectValue(v.value); old != nil {
		old.RemoveParent(v.parent, v.diffIndex)
	}
	v.value = value
	if object := objectValue(value); object != nil {
		object.InitLink(nil)
		object.AddParent(v.parent, v.diffIndex)
	}
}

func (v *Value[T]) SetValue(value T) bool {
	if equalValue(v.value, value) {
		return false
	}
	operation := PrimitiveSet
	if old := objectValue(v.value); old != nil {
		old.RemoveParent(v.parent, v.diffIndex)
		if any(value) == nil {
			operation = PointerClear
		}
	}
	v.value = value
	if object := objectValue(value); object != nil {
		object.InitLink(nil)
		object.AddParent(v.parent, v.diffIndex)
		operation = PointerSet
	} else if _, ok := any(value).(ObjectValue); ok {
		operation = PointerClear
	}
	v.parent.writeChildPatch(v.diffIndex, nil, operation, value)
	return true
}

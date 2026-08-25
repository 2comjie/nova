package diff_test

import (
	"github.com/2comjie/nova/diff"
)

type Player struct {
	diff.Object
	bag  diff.Pointer[*Bag]     `diff:"1"`
	age  diff.Primitive[int32]  `diff:"2"`
	name diff.Primitive[string] `diff:"3"`
}

type Bag struct {
	diff.Object
	OrderMap diff.PrimitiveMap[int32, int32] `diff:"1"`
	Hold     diff.Pointer[*Item]             `diff:"2"`
}

type Item struct {
	diff.Object
	ItemId     diff.Primitive[int32] `diff:"1"`
	ItemTypeId diff.Primitive[int32] `diff:"2"`
}

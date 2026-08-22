package diff

import (
	"errors"
	"testing"
)

type testParent struct {
	DirtyObject
	markedLinkIds []uint32
}

func (p *testParent) DiffObject() *DirtyObject {
	return &p.DirtyObject
}

func (p *testParent) MarkDiffChildDirty(linkId uint32) {
	p.markedLinkIds = append(p.markedLinkIds, linkId)
}

func TestBits(t *testing.T) {
	var bits Bits
	bits.Init(600)

	marked, firstDirty := bits.Mark(0, 1<<3)
	if !marked || !firstDirty {
		t.Fatal("第一次标脏结果错误")
	}

	marked, firstDirty = bits.Mark(0, 1<<3)
	if marked || firstDirty {
		t.Fatal("重复标脏结果错误")
	}

	marked, firstDirty = bits.Mark(9, 1<<23)
	if !marked || firstDirty {
		t.Fatal("overflow标脏结果错误")
	}
	if !bits.Has(0, 1<<3) || !bits.Has(9, 1<<23) {
		t.Fatal("脏标记不存在")
	}

	bits.Clear()
	if bits.Any() {
		t.Fatal("清理脏标记失败")
	}
}

func TestBitsDoesNotAllocateOverflowWithin64Fields(t *testing.T) {
	var bits Bits
	bits.Init(64)
	if bits.overflow != nil {
		t.Fatal("64个字段以内不应该分配overflow")
	}
}

func TestDirtyObjectInit(t *testing.T) {
	object := &DirtyObject{
		parents: []ParentLink{{linkId: 1}},
	}
	object.bits.Init(65)
	object.bits.Mark(1, 1)

	object.Init(65)

	if len(object.parents) != 0 {
		t.Fatal("初始化后不应该保留父引用")
	}
	if object.IsDirty() {
		t.Fatal("初始化后的对象不应该是脏的")
	}
	if object.bits.overflow == nil || len(*object.bits.overflow) != 1 {
		t.Fatal("65个字段应该分配一个overflow word")
	}
}

func TestDirtyObjectMultipleParents(t *testing.T) {
	root := &testParent{}
	root.Init(2)

	bag := &testParent{}
	bag.Init(1)
	bag.AddParent(root, 1)

	slot := &testParent{}
	slot.Init(1)
	slot.AddParent(root, 2)

	item := &DirtyObject{}
	item.Init(2)
	item.AddParent(bag, 10)
	item.AddParent(slot, 20)
	item.AddParent(slot, 20)

	if len(item.parents) != 2 {
		t.Fatalf("父引用数量错误: %d", len(item.parents))
	}

	item.Mark(0, 1)
	item.Mark(0, 2)
	if len(bag.markedLinkIds) != 1 || bag.markedLinkIds[0] != 10 {
		t.Fatalf("Bag收到的脏通知错误: %v", bag.markedLinkIds)
	}
	if len(slot.markedLinkIds) != 1 || slot.markedLinkIds[0] != 20 {
		t.Fatalf("Slot收到的脏通知错误: %v", slot.markedLinkIds)
	}

	item.ClearDirty()
	item.RemoveParent(slot, 20)
	item.Mark(0, 1)
	if len(bag.markedLinkIds) != 2 {
		t.Fatalf("Bag应该再次收到脏通知: %v", bag.markedLinkIds)
	}
	if len(slot.markedLinkIds) != 1 {
		t.Fatalf("解除父引用后Slot不应该收到通知: %v", slot.markedLinkIds)
	}
}

func TestLinkSessionSharedObject(t *testing.T) {
	root := &testParent{}
	bag := &testParent{}
	slot := &testParent{}
	item := &testParent{}

	var session LinkSession
	first, err := session.Enter(root.DiffObject(), 2, nil, 0)
	if err != nil || !first {
		t.Fatalf("Root进入引用图失败: first=%v err=%v", first, err)
	}

	first, err = session.Enter(bag.DiffObject(), 1, root, 1)
	if err != nil || !first {
		t.Fatalf("Bag进入引用图失败: first=%v err=%v", first, err)
	}
	first, err = session.Enter(item.DiffObject(), 1, bag, 10)
	if err != nil || !first {
		t.Fatalf("Item第一次进入引用图失败: first=%v err=%v", first, err)
	}
	session.Leave(item.DiffObject())
	session.Leave(bag.DiffObject())

	first, err = session.Enter(slot.DiffObject(), 1, root, 2)
	if err != nil || !first {
		t.Fatalf("Slot进入引用图失败: first=%v err=%v", first, err)
	}
	first, err = session.Enter(item.DiffObject(), 1, slot, 20)
	if err != nil || first {
		t.Fatalf("Item共享引用识别失败: first=%v err=%v", first, err)
	}
	session.Leave(slot.DiffObject())
	session.Leave(root.DiffObject())

	if len(item.parents) != 2 {
		t.Fatalf("Item父引用数量错误: %d", len(item.parents))
	}
}

func TestLinkSessionRejectsCircularReference(t *testing.T) {
	root := &testParent{}
	child := &testParent{}

	var session LinkSession
	if _, err := session.Enter(root.DiffObject(), 1, nil, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Enter(child.DiffObject(), 1, root, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Enter(root.DiffObject(), 1, child, 2); !errors.Is(err, ErrCircularReference) {
		t.Fatalf("循环引用检测结果错误: %v", err)
	}
}

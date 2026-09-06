//go:build !diff_fast

package basic

import (
	"testing"

	"github.com/2comjie/nova/diff"
)

func TestModel(t *testing.T) {
	writer := diff.NewWriter()
	value := new(Model)
	value.InitLink(writer)

	// 修改基础字段。
	if !value.SetLevel(10) || value.GetLevel() != 10 {
		t.Fatal("Level 赋值失败")
	}
	if writer.Len() != 1 {
		t.Fatal("没有记录 Level 变更")
	}

	// 相同值不产生新变更。
	if value.SetLevel(10) || writer.Len() != 1 {
		t.Fatal("相同值产生了变更")
	}

	// 同一个字段重复修改，合并为一条；零值也必须能记录。
	if !value.SetLevel(0) || value.GetLevel() != 0 || writer.Len() != 1 {
		t.Fatal("Level 零值修改或变更合并失败")
	}

	// 修改 Map。
	value.Scores().Store(7, 100)
	score, exists := value.Scores().Load(7)
	if !exists || score != 100 {
		t.Fatal("Scores 赋值失败")
	}
	if writer.Len() != 2 {
		t.Fatal("没有记录 Scores 变更")
	}

	writer.Reset()
	if writer.Len() != 0 || value.Scores().Len() != 1 {
		t.Fatal("Reset 应只清除变更记录，不清除业务数据")
	}
}

func TestChildReferences(t *testing.T) {
	writer := diff.NewWriter()
	value := new(Model)
	value.InitLink(writer)
	child := new(Child)
	value.SetChild(child)
	value.Children().Store(7, child)
	value.Order().Append(child)
	writer.Reset()

	child.SetCount(2)
	stored, exists := value.Children().Load(7)
	if value.GetChild() != child || !exists || stored != child || value.Order().GetValue(0) != child {
		t.Fatal("三种字段应引用同一个子对象")
	}
	if writer.Len() != 3 {
		t.Fatalf("子对象变更应沿三个引用记录，实际 %d 条", writer.Len())
	}

	value.SetChild(nil)
	writer.Reset()
	child.SetCount(3)
	if writer.Len() != 2 {
		t.Fatalf("解除普通指针后应保留两个引用，实际 %d 条", writer.Len())
	}
}

package diffgen

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	dir := t.TempDir()
	source := `package model

import (
	"github.com/2comjie/nova/diff"
	bag "github.com/acme/game/bag"
)

type Player[T ~int32] struct {
	diff.Object
	Level diff.Primitive[T] ` + "`diff:\"1\"`" + `
	Bag diff.Pointer[*bag.Bag] ` + "`diff:\"2\"`" + `
	Scores diff.PrimitiveMap[int32, int64] ` + "`diff:\"3\"`" + `
	Items diff.PointerMap[uint64, *bag.Item] ` + "`diff:\"4\"`" + `
	Order diff.PrimitiveSlice[uint64] ` + "`diff:\"5\"`" + `
	Slots diff.PointerSlice[*bag.Item] ` + "`diff:\"6\"`" + `
}
`
	if err := os.WriteFile(filepath.Join(dir, "player.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := Generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "player_diff.gen.go" {
		t.Fatalf("files = %v", files)
	}

	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, expected := range []string{
		"func (value *Player[T]) InitLink(writer *diff.Writer)",
		"func (value *Player[T]) EnsureDiffLink()",
		"func (value *Player[T]) InitDiffLink(writer *diff.Writer, visited map[*diff.Object]struct{})",
		"child.InitDiffLink(nil, visited)",
		"value.Level.Init(&value.Object, 1)",
		"value.Items.Range(func(_ uint64, child *bag.Item) bool",
		"value.Slots.Range(func(_ int, child *bag.Item) bool",
		"func (value *Player[T]) AppendDiffValue(data []byte) []byte",
		"data = value.Slots.AppendValue(data, 6)",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("generated file does not contain %q\n%s", expected, content)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), files[0], data, parser.AllErrors); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateSchema(t *testing.T) {
	dir := t.TempDir()
	source := `//go:build diff_fast

package model

type Player[T ~int32] struct {
	Level T ` + "`diff:\"1\"`" + `
	Bag *Bag ` + "`diff:\"2\"`" + `
	Scores map[int32]int64 ` + "`diff:\"3\"`" + `
	Items map[uint64]*Item ` + "`diff:\"4\"`" + `
	Order []uint64 ` + "`diff:\"5\"`" + `
	Slots []*Item ` + "`diff:\"6\"`" + `
}

type Bag struct {
	Hold *Item ` + "`diff:\"1\"`" + `
}

type Item struct {
	Count int32 ` + "`diff:\"1\"`" + `
}
`
	if err := os.WriteFile(filepath.Join(dir, "a_data.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := Generate(dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, expected := range []string{
		"//go:build !diff_fast",
		"type Player[T ~int32] struct",
		"diff.Primitive[T]",
		"diff.Pointer[*Bag]",
		"diff.PrimitiveMap[int32, int64]",
		"diff.PointerMap[uint64, *Item]",
		"diff.PrimitiveSlice[uint64]",
		"diff.PointerSlice[*Item]",
		"func (value *Player[T]) GetLevel() T",
		"func (value *Player[T]) SetLevel(fieldValue T) bool",
		"func (value *Player[T]) GetBag() *Bag",
		"func (value *Player[T]) SetBag(fieldValue *Bag) bool",
		"func (value *Player[T]) ClearBag() bool",
		"func (value *Player[T]) Items() *diff.PointerMap[uint64, *Item]",
		"func (value *Player[T]) Commit() []byte",
		"func (value *Player[T]) Snapshot() []byte",
		"func (value *Player[T]) LoadSnapshot(data []byte) error",
		"func (value *Player[T]) Merge(data []byte) error",
		"func (value *Player[T]) MergeDiffPatch(path []diff.EncodedPathNode, operation diff.Operation, data []byte) error",
		"type PlayerDiffPath[DiffRoot any, T ~int32] struct",
		"func PlayerDiff[T ~int32]() PlayerDiffPath[*Player[T], T]",
		"func (path PlayerDiffPath[DiffRoot, T]) Level() diff.ValuePath[DiffRoot, T]",
		"func (path PlayerDiffPath[DiffRoot, T]) Items() PlayerItemsDiffPath[DiffRoot, T]",
		"func (path PlayerItemsDiffPath[DiffRoot, T]) Any() ItemDiffPath[DiffRoot]",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("generated schema does not contain %q\n%s", expected, content)
		}
	}
}

func TestGenerateRejectsInvalidModel(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "missing object",
			source: `package model
import "github.com/2comjie/nova/diff"
type Player struct { Age diff.Primitive[int32] ` + "`diff:\"1\"`" + ` }
`,
			want: "必须匿名嵌入diff.Object",
		},
		{
			name: "duplicate index",
			source: `package model
import "github.com/2comjie/nova/diff"
type Player struct {
	diff.Object
	Age diff.Primitive[int32] ` + "`diff:\"1\"`" + `
	Name diff.Primitive[string] ` + "`diff:\"1\"`" + `
}
`,
			want: "相同的diff标签1",
		},
		{
			name: "pointer value",
			source: `package model
import "github.com/2comjie/nova/diff"
type Player struct {
	diff.Object
	Bag diff.Pointer[Bag] ` + "`diff:\"1\"`" + `
}
type Bag struct { diff.Object }
`,
			want: "Pointer值必须是指针类型",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "model.go"), []byte(test.source), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Generate(dir)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestGeneratedCodeCompilesAndRestoresLinks(t *testing.T) {
	dir := t.TempDir()
	_, currentFile, _, _ := runtime.Caller(0)
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../.."))
	goMod := "module example.com/diffgentest\n\ngo 1.24.3\n\nrequire github.com/2comjie/nova v0.0.0\n\nreplace github.com/2comjie/nova => " + moduleRoot + "\n"
	model := `//go:build diff_fast

package model

type Player struct {
	Level int32 ` + "`diff:\"1\"`" + `
	Bag *Bag ` + "`diff:\"2\"`" + `
	Scores map[uint64]int32 ` + "`diff:\"3\"`" + `
	Numbers []int32 ` + "`diff:\"4\"`" + `
}

type Bag struct {
	Items map[uint64]*Item ` + "`diff:\"1\"`" + `
	Order []*Item ` + "`diff:\"2\"`" + `
}

type Item struct {
	Count int32 ` + "`diff:\"1\"`" + `
}
`
	modelTest := `package model

import (
	"testing"
	"github.com/2comjie/nova/diff"
)

var afterCount int

func init() {
	diff.ListenBefore(PlayerDiff.Level(), func(change *diff.Change[int32]) {
		if change.NewValue < 0 {
			change.Cancel("level小于0")
			return
		}
		if change.NewValue > 100 {
			change.Replace(100)
		}
	})
	diff.ListenBefore(PlayerDiff.Bag().Items().Any().Count(), func(change *diff.Change[int32]) {
		if change.NewValue > 20 {
			change.Replace(20)
		}
	})
	diff.ListenBefore(PlayerDiff.Bag().Order().Any().Count(), func(change *diff.Change[int32]) {
		if change.NewValue > 15 {
			change.Replace(15)
		}
	})
	diff.ListenBefore(PlayerDiff.Bag().Changes(), func(change *diff.Change[*Bag]) {
		if change.NewValue == nil {
			change.Cancel("bag不能为空")
		}
	})
	diff.ListenMapBefore(PlayerDiff.Scores(), func(change *diff.MapChange[uint64, int32]) {
		if change.HasKey && change.Key == 1 && change.NewExists && change.NewValue > 200 {
			change.Replace(200)
		}
	})
	diff.ListenMapBefore(PlayerDiff.Scores().Key(2), func(change *diff.MapChange[uint64, int32]) {
		change.Cancel("key 2不可写")
	})
	diff.ListenMapBefore(PlayerDiff.Bag().Items().Changes(), func(change *diff.MapChange[uint64, *Item]) {
		if change.HasKey && change.Key == 8 {
			change.Cancel("key 8不可写")
		}
	})
	diff.ListenSliceBefore(PlayerDiff.Numbers(), func(change *diff.SliceChange[int32]) {
		if change.HasNew && change.NewValue > 10 {
			change.Replace(10)
		}
	})
	diff.ListenSliceBefore(PlayerDiff.Numbers().Index(1), func(change *diff.SliceChange[int32]) {
		if change.HasNew {
			change.Replace(9)
		}
	})
	diff.ListenSliceBefore(PlayerDiff.Bag().Order().Changes(), func(change *diff.SliceChange[*Item]) {
		if change.Operation == diff.ChangeSliceAppend && change.NewValue == nil {
			change.Cancel("nil item")
		}
	})
	diff.ListenAfter(PlayerDiff.Bag().Items().Any().Count(), func(change diff.Change[int32]) {
		afterCount++
	})
}

func TestLinks(t *testing.T) {
	afterCount = 0
	item := &Item{}
	item.SetCount(1)
	bag := &Bag{}
	bag.Items().Store(7, item)
	bag.Order().Append(item)
	player := &Player{}
	writer := diff.NewWriter()
	player.InitLink(writer)
	player.SetLevel(10)
	player.SetBag(bag)
	player.Scores().Store(1, 100)
	player.Numbers().Append(3)
	player.Commit()

	snapshot := &Player{}
	if err := snapshot.LoadSnapshot(player.Snapshot()); err != nil {
		t.Fatal(err)
	}
	if snapshot.GetLevel() != 10 || snapshot.GetBag() == nil {
		t.Fatal("invalid snapshot")
	}

	if player.SetLevel(-1) {
		t.Fatal("canceled level was changed")
	}
	if player.ClearBag() || player.GetBag() == nil {
		t.Fatal("canceled bag clear was applied")
	}
	blockedItem := &Item{}
	if player.GetBag().Items().Store(8, blockedItem) {
		t.Fatal("canceled pointer map store was applied")
	}
	if player.Scores().Store(2, 1) {
		t.Fatal("canceled primitive map store was applied")
	}
	player.GetBag().Order().Append(nil)
	if player.GetBag().Order().Len() != 1 {
		t.Fatal("canceled pointer slice append was applied")
	}
	player.SetLevel(101)
	item.SetCount(30)
	player.Scores().Store(1, 201)
	player.Numbers().Append(11)
	delta := player.Commit()
	if writer.Len() != 0 {
		t.Fatalf("writer was not reset")
	}
	if err := snapshot.Merge(delta); err != nil {
		t.Fatal(err)
	}
	mapItem, exists := snapshot.GetBag().Items().Load(7)
	if !exists || mapItem.GetCount() != 15 {
		t.Fatal("pointer map was not merged")
	}
	if snapshot.GetLevel() != 100 {
		t.Fatal("primitive was not merged")
	}
	score, exists := snapshot.Scores().Load(1)
	if !exists || score != 200 {
		t.Fatal("primitive map was not merged")
	}
	if snapshot.Numbers().Len() != 2 || snapshot.Numbers().GetValue(1) != 9 {
		t.Fatal("slice was not merged")
	}
	if afterCount != 1 {
		t.Fatalf("after count = %d", afterCount)
	}
}
`
	files := map[string]string{
		"go.mod":        goMod,
		"model.go":      model,
		"model_test.go": modelTest,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Generate(dir); err != nil {
		t.Fatal(err)
	}

	command := exec.Command("go", "test", ".")
	command.Dir = dir
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go test: %v\n%s", err, output)
	}
}

func TestGeneratedGenericListenerPathCompiles(t *testing.T) {
	dir := t.TempDir()
	_, currentFile, _, _ := runtime.Caller(0)
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../.."))
	goMod := "module example.com/diffgengeneric\n\ngo 1.24.3\n\nrequire github.com/2comjie/nova v0.0.0\n\nreplace github.com/2comjie/nova => " + moduleRoot + "\n"
	model := `//go:build diff_fast

package model

type Box[T ~int32] struct {
	Value T ` + "`diff:\"1\"`" + `
	Child *Node[T] ` + "`diff:\"2\"`" + `
}

type Node[T ~int32] struct {
	Value T ` + "`diff:\"1\"`" + `
}
`
	modelTest := `package model

import (
	"testing"
	"github.com/2comjie/nova/diff"
)

func init() {
	diff.ListenBefore(BoxDiff[int32]().Child().Value(), func(change *diff.Change[int32]) {
		if change.NewValue > 10 {
			change.Replace(10)
		}
	})
}

func TestGenericPath(t *testing.T) {
	child := &Node[int32]{}
	box := &Box[int32]{}
	box.InitLink(diff.NewWriter())
	box.SetChild(child)
	child.SetValue(20)
	if child.GetValue() != 10 {
		t.Fatalf("value = %d", child.GetValue())
	}
}
`
	for name, content := range map[string]string{
		"go.mod":        goMod,
		"model.go":      model,
		"model_test.go": modelTest,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Generate(dir); err != nil {
		t.Fatal(err)
	}

	command := exec.Command("go", "test", ".")
	command.Dir = dir
	command.Env = append(os.Environ(), "GOWORK=off")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("go test: %v\n%s", err, output)
	}
}

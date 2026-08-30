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

type Player struct {
	diff.Object
	Level diff.Primitive[int32] ` + "`diff:\"1\"`" + `
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
		"func (value *Player) InitLink(writer *diff.Writer)",
		"func (value *Player) EnsureDiffLink()",
		"func (value *Player) InitDiffLink(writer *diff.Writer, visited map[*diff.Object]struct{})",
		"child.InitDiffLink(nil, visited)",
		"value.Level.Init(&value.Object, 1)",
		"value.Items.Range(func(_ uint64, child *bag.Item) bool",
		"value.Slots.Range(func(_ int, child *bag.Item) bool",
		"func (value *Player) AppendDiffValue(data []byte) []byte",
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

import "github.com/2comjie/nova/logx/logdef"

type Player struct {
	logdef.ILogger
	RuntimeName string ` + "`diff:\"-\"`" + `
	Level int32 ` + "`diff:\"1\"`" + `
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
	if len(files) != 3 {
		t.Fatalf("files = %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, expected := range []string{
		"//go:build !diff_fast",
		"type Player struct",
		"logdef.ILogger",
		"RuntimeName",
		"diff.Primitive[int32]",
		"diff.Pointer[*Bag]",
		"diff.PrimitiveMap[int32, int64]",
		"diff.PointerMap[uint64, *Item]",
		"diff.PrimitiveSlice[uint64]",
		"diff.PointerSlice[*Item]",
		"func (value *Player) GetLevel() int32",
		"func (value *Player) SetLevel(fieldValue int32) bool",
		"func (value *Player) GetBag() *Bag",
		"func (value *Player) SetBag(fieldValue *Bag) bool",
		"func (value *Player) ClearBag() bool",
		"func (value *Player) Items() *diff.PointerMap[uint64, *Item]",
		"func (value *Player) Commit() diff.Delta[*Player]",
		"func (value *Player) FormatDelta(data []byte) (string, error)",
		"func (value *Player) Snapshot() []byte",
		"func (value *Player) LoadSnapshot(data []byte) error",
		"func (value *Player) Merge(data []byte) error",
		"func (value *Player) MergeDiffPatch(path []diff.EncodedPathNode, operation diff.Operation, data []byte) error",
		"type PlayerDiffPath[DiffRoot any] struct",
		"var PlayerDiff = NewPlayerDiffPath[*Player]",
		"func (path PlayerDiffPath[DiffRoot]) Level() diff.ValuePath[DiffRoot, int32]",
		"func (path PlayerDiffPath[DiffRoot]) Items() PlayerItemsDiffPath[DiffRoot]",
		"func (path PlayerItemsDiffPath[DiffRoot]) Any() ItemDiffPath[DiffRoot]",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("generated schema does not contain %q\n%s", expected, content)
		}
	}

	protoData, err := os.ReadFile(files[1])
	if err != nil {
		t.Fatal(err)
	}
	protoContent := string(protoData)
	for _, expected := range []string{
		"message Player {",
		"int32 level = 1;",
		"Bag bag = 2;",
		"map<int32, int64> scores = 3;",
		"map<uint64, Item> items = 4;",
		"repeated uint64 order = 5;",
		"repeated Item slots = 6;",
	} {
		if !strings.Contains(protoContent, expected) {
			t.Fatalf("generated data proto does not contain %q\n%s", expected, protoContent)
		}
	}

	diffData, err := os.ReadFile(files[2])
	if err != nil {
		t.Fatal(err)
	}
	diffContent := string(diffData)
	for _, expected := range []string{
		"message PathNode {",
		"oneof map_key {",
		"message Patch {",
		"oneof value {",
		"int32 int32_value",
		"ItemList item_list_value",
		"message PlayerSyncPush {",
		"oneof payload {",
		".model.Player full = 10;",
		"Delta delta = 11;",
	} {
		if !strings.Contains(diffContent, expected) {
			t.Fatalf("generated diff proto does not contain %q\n%s", expected, diffContent)
		}
	}

	if protoc, err := exec.LookPath("protoc"); err == nil {
		command := exec.Command(protoc, "--proto_path", dir, "--descriptor_set_out", filepath.Join(dir, "schema.pb"), files[1], files[2])
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("protoc: %v\n%s", err, output)
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
	goMod := "module example.com/diffgentest\n\ngo 1.27.0\n\nrequire github.com/2comjie/nova v0.0.0\n\nreplace github.com/2comjie/nova => " + moduleRoot + "\n"
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
	"strings"
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
	debugText := delta.String()
	if !strings.Contains(debugText, "Player.Level Set = 100") || !strings.Contains(debugText, "Player.Scores[1] MapSet = 200") {
		t.Fatalf("delta debug = %s", debugText)
	}
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

func TestGenerateRejectsGenericSchema(t *testing.T) {
	dir := t.TempDir()
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
	if err := os.WriteFile(filepath.Join(dir, "model.go"), []byte(model), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Generate(dir)
	if err == nil || !strings.Contains(err.Error(), "diff数据类型不支持泛型") {
		t.Fatalf("err = %v", err)
	}
}

package diffgen

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSnapshot(t *testing.T) {
	t.Setenv("GOWORK", "off")
	moduleDir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	for _, dir := range []string{"diff", "generic", "internal/diffgen/testdata"} {
		if err := os.CopyFS(filepath.Join(workDir, dir), os.DirFS(filepath.Join(moduleDir, dir))); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		data, err := os.ReadFile(filepath.Join(moduleDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(workDir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	protoDir := filepath.Join(workDir, "proto")
	for _, name := range []string{"basic", "external"} {
		if err := Generate(filepath.Join(workDir, "internal/diffgen/testdata", name), protoDir); err != nil {
			t.Fatal(err)
		}
	}
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
	plugin := filepath.Join(workDir, "protoc-gen-go")
	command := exec.Command(goBin, "build", "-mod=readonly", "-o", plugin, "google.golang.org/protobuf/cmd/protoc-gen-go")
	command.Dir = workDir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build protoc-gen-go: %v\n%s", err, output)
	}
	command = exec.Command("protoc",
		"--plugin=protoc-gen-go="+plugin,
		"--proto_path="+protoDir,
		"--go_out="+workDir,
		"--go_opt=module=github.com/2comjie/nova",
		"basic/child.proto", "basic/model.proto", "basic/scalars.proto", "basic/collections.proto", "external/model.proto",
	)
	command.Dir = workDir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generate protobuf: %v\n%s", err, output)
	}

	const snapshotTest = `package external

import (
    "testing"

    "github.com/2comjie/nova/diff"
    "github.com/2comjie/nova/internal/diffgen/testdata/basic"
    pbBasic "github.com/2comjie/nova/pb/basic"
    pbData "github.com/2comjie/nova/pb/external"
    "google.golang.org/protobuf/proto"
)

func TestSnapshot(t *testing.T) {
    empty := new(Model)
    if got := empty.Snapshot(); got.Child != nil || got.Basic != nil {
        t.Fatal("empty pointers must remain absent", got)
    }
    if empty.Object.Initialized() {
        t.Fatal("snapshot initialized the model")
    }

    child := new(basic.Child)
    child.SetCount(7)
    nested := new(basic.Model)
    nested.SetLevel(20)
    nested.SetChild(child)
    nested.Scores().Store(1001, 12)
    nested.Children().Store(1001, child)
    nested.Order().Append(child)

    writer := diff.NewWriter()
    value := new(Model)
    value.InitLink(writer)
    value.SetChild(child)
    value.SetBasic(nested)
    value.Children().Store(1001, child)
    value.Order().Append(child)
    value.Order().Append(nil)
    value.Order().Append(child)
    patches := writer.Len()

    snapshot := value.Snapshot()
    if snapshot.Child.Count != 7 || snapshot.Basic.Level != 20 || snapshot.Basic.Child.Count != 7 {
        t.Fatal("missing nested state", snapshot)
    }
    if snapshot.Child == snapshot.Basic.Child {
        t.Fatal("shared runtime child leaked into the snapshot")
    }
    if snapshot.Children[1001].Count != 7 || snapshot.Order[0].Count != 7 ||
        len(snapshot.Order) != 3 || snapshot.Order[1] != nil || snapshot.Order[2].Count != 7 ||
        snapshot.Basic.Scores[1001] != 12 || snapshot.Basic.Children[1001].Count != 7 ||
        snapshot.Basic.Order[0].Count != 7 {
        t.Fatal("missing collection state", snapshot)
    }
    if snapshot.Children[1001] == snapshot.Order[0] || snapshot.Order[0] == snapshot.Order[2] {
        t.Fatal("snapshot collection entries share mutable messages")
    }
    if writer.Len() != patches {
        t.Fatal("snapshot changed the writer")
    }

    body, err := proto.Marshal(snapshot)
    if err != nil {
        t.Fatal(err)
    }
    decoded := new(pbData.Model)
    if err := proto.Unmarshal(body, decoded); err != nil {
        t.Fatal(err)
    }
    if !proto.Equal(snapshot, decoded) {
        t.Fatal("snapshot changed after protobuf round trip")
    }

    child.SetCount(9)
    if snapshot.Child.Count != 7 || snapshot.Basic.Child.Count != 7 ||
        snapshot.Children[1001].Count != 7 || snapshot.Order[0].Count != 7 {
        t.Fatal("model mutation changed the old snapshot")
    }
    snapshot.Basic.Child.Count = 30
    if child.GetCount() != 9 || snapshot.Child.Count != 7 {
        t.Fatal("snapshot mutation changed the model or another snapshot branch")
    }
    if value.Snapshot().Basic.Child.Count != 9 {
        t.Fatal("fresh snapshot does not contain the latest state")
    }
    snapshot.Children[1001].Count = 40
    snapshot.Order[0].Count = 50
    snapshot.Basic.Scores[1001] = 60
    score, _ := nested.Scores().Load(1001)
    if child.GetCount() != 9 || score != 12 || snapshot.Order[2].Count != 7 {
        t.Fatal("collection snapshot mutated the source or another entry")
    }

    value.Children().Delete(1001)
    value.Order().Clear()
    nested.Scores().Clear()
    patches = writer.Len()
    cleared := value.Snapshot()
    if len(cleared.Children) != 0 || len(cleared.Order) != 0 || len(cleared.Basic.Scores) != 0 ||
        writer.Len() != patches || len(snapshot.Children) != 1 || len(snapshot.Order) != 3 {
        t.Fatal("clear lost patches or changed an older snapshot")
    }

    value.SetChild(nil)
    patches = writer.Len()
    if value.Snapshot().Child != nil || writer.Len() != patches {
        t.Fatal("cleared pointer was restored or its patch was lost")
    }
}

func TestPrimitiveCollectionsSnapshot(t *testing.T) {
    value := new(basic.Collections)
    if !proto.Equal(value.Snapshot(), new(pbBasic.Collections)) || value.Object.Initialized() {
        t.Fatal("empty snapshot must not initialize the model")
    }
    writer := diff.NewWriter()
    value.InitLink(writer)
    value.Signed().Store(-128, -32768)
    value.Unsigned().Store(65535, 255)
    value.Flags().Store(false, "")
    value.Flags().Store(true, "yes")
    value.Floats().Store("speed", 1.5)
    value.Doubles().Append(2.25)
    value.Doubles().Append(-3.5)
    value.Counts().Append(-7)
    value.Bytes().Append(255)
    patches := writer.Len()

    snapshot := value.Snapshot()
    expected := &pbBasic.Collections{
        Signed: map[int32]int32{-128: -32768},
        Unsigned: map[uint32]uint32{65535: 255},
        Flags: map[bool]string{false: "", true: "yes"},
        Floats: map[string]float32{"speed": 1.5},
        Doubles: []float64{2.25, -3.5},
        Counts: []int64{-7},
        Bytes: []uint32{255},
    }
    if !proto.Equal(snapshot, expected) || writer.Len() != patches {
        t.Fatal("collection conversion lost data or changed patches", snapshot)
    }
    body, err := proto.Marshal(snapshot)
    if err != nil {
        t.Fatal(err)
    }
    decoded := new(pbBasic.Collections)
    if err := proto.Unmarshal(body, decoded); err != nil {
        t.Fatal(err)
    }
    if !proto.Equal(snapshot, decoded) {
        t.Fatal("collections changed after protobuf round trip")
    }

    snapshot.Signed[-128] = 1
    snapshot.Doubles[0] = 9
    original, _ := value.Signed().Load(-128)
    if original != -32768 || value.Doubles().GetValue(0) != 2.25 || writer.Len() != patches {
        t.Fatal("snapshot changed the runtime collections")
    }
    value.Floats().Store("speed", 8)
    value.Bytes().SetValue(0, 1)
    if snapshot.Floats["speed"] != 1.5 || snapshot.Bytes[0] != 255 {
        t.Fatal("runtime mutation changed the snapshot")
    }
    value.Flags().Delete(true)
    value.Counts().Clear()
    patches = writer.Len()
    latest := value.Snapshot()
    if _, exists := latest.Flags[true]; exists {
        t.Fatal("deleted map entry returned in snapshot")
    }
    if len(latest.Counts) != 0 || len(snapshot.Counts) != 1 || writer.Len() != patches {
        t.Fatal("cleared slice returned or patches changed")
    }
}
`
	if err := os.WriteFile(filepath.Join(workDir, "internal/diffgen/testdata/external/snapshot_test.go"), []byte(snapshotTest), 0644); err != nil {
		t.Fatal(err)
	}
	command = exec.Command(goBin, "test", "-mod=readonly", "-count=1",
		"./internal/diffgen/testdata/basic", "./internal/diffgen/testdata/external")
	command.Dir = workDir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("test generated snapshots: %v\n%s", err, output)
	}
}

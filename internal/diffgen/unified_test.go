package diffgen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnifiedGenericGeneration(t *testing.T) {
	dir := t.TempDir()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	files := map[string]string{
		"go.mod":       fmt.Sprintf("module example.com/unified\n\ngo 1.27.0\n\nrequire github.com/2comjie/nova v0.0.0\nreplace github.com/2comjie/nova => %q\n", root),
		"diffgen.yaml": "source_root: ./models\npackages: [./models]\nproto:\n  dir: ./proto\n  go_out: .\n",
		"models/data.go": `//go:build diff_fast

package models

type Foo[K comparable, V any] struct {
 Value V ` + "`diff:\"1\"`" + `
 Items map[K]V ` + "`diff:\"2\"`" + `
 History []V ` + "`diff:\"3\"`" + `
}

type Scalar Foo[string, int64]
type Pointer Foo[int64, *Child]
type Child struct { Count int32 ` + "`diff:\"1\"`" + ` }
`,
	}
	for name, data := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := Generate(filepath.Join(dir, "diffgen.yaml")); err != nil {
			t.Fatal(err)
		}
	}
	code, err := os.ReadFile(filepath.Join(dir, "models/data_diff.gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type Foo[K comparable, V any] struct", "diff.Value[V]", "diff.Map[K, V]", "diff.Slice[V]",
		"type Scalar = Foo[string, int64]", "type Pointer = Foo[int64, *Child]",
		"(*diff.Value[int64])", "(*diff.Value[*Child])",
		"(*diff.Map[string, int64])", "(*diff.Map[int64, *Child])",
	} {
		if !strings.Contains(string(code), want) {
			t.Errorf("生成代码缺少 %s", want)
		}
	}
	protocol, err := os.ReadFile(filepath.Join(dir, "proto/data.proto"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"message Scalar", "message Pointer", "map<string, int64>", "map<int64, Child>"} {
		if !strings.Contains(string(protocol), want) {
			t.Errorf("Proto 缺少 %s", want)
		}
	}
	if strings.Contains(string(protocol), "message Foo") {
		t.Fatal("泛型模板不应生成 Proto")
	}
	if _, err := os.Stat(filepath.Join(dir, "pb")); !os.IsNotExist(err) {
		t.Fatal("生成器不应自动编译 Proto", err)
	}
}

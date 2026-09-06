package diffgen

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	protoDir := t.TempDir()
	for _, dir := range []string{"testdata/basic", "testdata/external"} {
		if err := Generate(dir, protoDir); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"basic/model.proto", "external/model.proto"} {
		content, err := os.ReadFile(filepath.Join(protoDir, name))
		if err != nil {
			t.Fatal(err)
		}
		goPackage := `option go_package = "github.com/2comjie/nova/pb/` + filepath.Dir(name) + `";`
		if !strings.Contains(string(content), goPackage) {
			t.Fatalf("%s 缺少 go_package %s", name, goPackage)
		}
		if strings.Count(string(content), `import "basic/child.proto";`) != 1 {
			t.Fatalf("%s 应只导入一次 basic/child.proto:\n%s", name, content)
		}
		if name == "external/model.proto" {
			if !strings.Contains(string(content), "basic.Child child = 1;") {
				t.Fatalf("跨包 Proto 类型不能使用 Go import 别名:\n%s", content)
			}
			if !strings.Contains(string(content), "import \"basic/child.proto\";\n\nimport \"basic/model.proto\";") {
				t.Fatalf("Proto imports 应去重并排序:\n%s", content)
			}
		}
	}

	pluginPath := filepath.Join(t.TempDir(), "protoc-gen-go")
	build := exec.CommandContext(
		t.Context(),
		filepath.Join(runtime.GOROOT(), "bin", "go"),
		"build", "-o", pluginPath,
		"google.golang.org/protobuf/cmd/protoc-gen-go",
	)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("构建 Proto 插件失败: %v\n%s", err, output)
	}

	goDir := t.TempDir()
	command := exec.CommandContext(
		t.Context(),
		"protoc",
		"--plugin=protoc-gen-go="+pluginPath,
		"--proto_path="+protoDir,
		"--go_out="+goDir,
		"--go_opt=module=github.com/2comjie/nova",
		"basic/child.proto",
		"basic/model.proto",
		"external/model.proto",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("生成 Go 消息失败: %v\n%s", err, output)
	}
	for _, name := range []string{"pb/basic/child.pb.go", "pb/basic/model.pb.go", "pb/external/model.pb.go"} {
		if _, err := os.Stat(filepath.Join(goDir, name)); err != nil {
			t.Fatal(err)
		}
	}
}

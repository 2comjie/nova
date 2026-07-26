package main

import (
	"bytes"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunNewUsesDefaultWaliVersion(t *testing.T) {
	root := filepath.Join(t.TempDir(), "game")
	var output strings.Builder
	if err := runNew([]string{
		"github.com/example/game",
		"--dir=" + root,
	}, &output); err != nil {
		t.Fatalf("runNew() error = %v", err)
	}
	assertFileContains(t,
		filepath.Join(root, "go.mod"),
		"github.com/2comjie/wali v0.1.0",
	)
}

func TestProjectGenerateNodeAndRoutes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "game")
	createdRoot, err := newProject(
		"github.com/example/game",
		root,
		"v0.1.0",
	)
	if err != nil {
		t.Fatalf("newProject() error = %v", err)
	}
	if createdRoot != root {
		t.Fatalf("newProject() root = %q, want %q", createdRoot, root)
	}
	assertFileContains(t,
		filepath.Join(root, "go.mod"),
		"github.com/2comjie/wali v0.1.0",
	)
	assertFileContains(t,
		filepath.Join(root, "go.mod"),
		"github.com/2comjie/wali/locator/redis v0.1.0",
	)
	assertFileContains(t,
		filepath.Join(root, "go.mod"),
		"github.com/2comjie/wali/registry/redis v0.1.0",
	)
	assertFileContains(t,
		filepath.Join(root, "go.mod"),
		"github.com/redis/go-redis/v9 v9.21.0",
	)

	scriptPath := filepath.Join(root, "proto_gen.sh")
	scriptInfo, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("stat proto_gen.sh error = %v", err)
	}
	if scriptInfo.Mode().Perm()&0o111 == 0 {
		t.Fatalf("proto_gen.sh mode = %v, want executable", scriptInfo.Mode())
	}

	if err := addNode(root, "chat"); err != nil {
		t.Fatalf("addNode() error = %v", err)
	}
	if err := addNode(root, "room"); err != nil {
		t.Fatalf("addNode(room) error = %v", err)
	}
	assertFileContains(t,
		filepath.Join(root, "api/server/chat/chat.proto"),
		"package game.server.chat;",
	)
	assertFileContains(t,
		filepath.Join(root, "api/server/chat/chat.proto"),
		"go_package = \"github.com/example/game/internal/pb/chat;chatpb\";",
	)
	assertFileContains(t,
		filepath.Join(root, "internal/chat/rpc.go"),
		"chatpb.RegisterChatServer",
	)
	assertFileContains(t,
		filepath.Join(root, "cmd/chat/main.go"),
		"service.RegisterRPC(rpcServer)",
	)
	assertFileContains(t,
		filepath.Join(root, "internal/rpcclient/client_gen.go"),
		"ChatClient chatpb.ChatClient",
	)
	assertFileContains(t,
		filepath.Join(root, "internal/rpcclient/client_gen.go"),
		"RoomClient roompb.RoomClient",
	)
	assertFileContains(t,
		filepath.Join(root, "internal/rpcclient/client_gen.go"),
		"ChatClient = chatpb.NewChatClient(base)",
	)
	assertFileContains(t,
		filepath.Join(root, "internal/rpcclient/client_gen.go"),
		"RoomClient = roompb.NewRoomClient(base)",
	)
	assertFileContains(t,
		filepath.Join(root, "internal/rpcclient/client_gen.go"),
		"initOnce.Do(func()",
	)
	assertFileNotContains(t,
		filepath.Join(root, "internal/rpcclient/client_gen.go"),
		"type Client struct",
	)
	if _, err := os.Stat(filepath.Join(root, "api/server/chat/v1")); !os.IsNotExist(err) {
		t.Fatalf("不应生成api/server/chat/v1目录, stat error = %v", err)
	}
	assertFileContains(t,
		filepath.Join(root, "internal/bootstrap/infrastructure.go"),
		"RPCClient *walirpc.Client",
	)
	assertFileContains(t,
		filepath.Join(root, "internal/bootstrap/infrastructure.go"),
		"projectrpc.Init(baseRPCClient)",
	)

	if err := addRoute(root, RouteSpec{
		Name:  "chat.send",
		ID:    1001,
		Node:  "chat",
		Reply: true,
	}); err != nil {
		t.Fatalf("addRoute(chat.send) error = %v", err)
	}
	assertFileContains(t,
		filepath.Join(root, "internal/gen/route/routes_gen.go"),
		"ChatSend uint32 = 1001",
	)
	assertFileContains(t,
		filepath.Join(root, "internal/chat/routes_gen.go"),
		"router.Handle(route.ChatSend, handleChatSend)",
	)
	assertFileContains(t,
		filepath.Join(root, "configs/gate/routes_gen.yaml"),
		"service: chat",
	)

	handlerPath := filepath.Join(root, "internal/chat/handler_chat_send.go")
	const customHandler = "package chat\n\n// 用户已经修改过的Handler。\n"
	if err := os.WriteFile(handlerPath, []byte(customHandler), 0o644); err != nil {
		t.Fatalf("write custom handler error = %v", err)
	}
	if err := addRoute(root, RouteSpec{
		Name:  "chat.typing",
		ID:    1002,
		Node:  "chat",
		Reply: false,
	}); err != nil {
		t.Fatalf("addRoute(chat.typing) error = %v", err)
	}
	handlerBody, err := os.ReadFile(handlerPath)
	if err != nil {
		t.Fatalf("read custom handler error = %v", err)
	}
	if string(handlerBody) != customHandler {
		t.Fatal("生成新Route时覆盖了用户Handler")
	}

	assertGeneratedGoFormatted(t, root)
}

func TestAddRouteRejectsDuplicateAndUnknownNode(t *testing.T) {
	root := filepath.Join(t.TempDir(), "game")
	if _, err := newProject("github.com/example/game", root, "v0.1.0"); err != nil {
		t.Fatalf("newProject() error = %v", err)
	}
	if err := addNode(root, "chat"); err != nil {
		t.Fatalf("addNode() error = %v", err)
	}
	route := RouteSpec{Name: "chat.send", ID: 1001, Node: "chat", Reply: true}
	if err := addRoute(root, route); err != nil {
		t.Fatalf("addRoute() error = %v", err)
	}
	if err := addRoute(root, route); err == nil ||
		!strings.Contains(err.Error(), "Route名称已经存在") {
		t.Fatalf("duplicate name error = %v", err)
	}
	if err := addRoute(root, RouteSpec{
		Name: "chat.history",
		ID:   1001,
		Node: "chat",
	}); err == nil || !strings.Contains(err.Error(), "Route ID已经存在") {
		t.Fatalf("duplicate ID error = %v", err)
	}
	if err := addRoute(root, RouteSpec{
		Name: "player.get",
		ID:   2001,
		Node: "player",
	}); err == nil || !strings.Contains(err.Error(), "Node不存在") {
		t.Fatalf("unknown Node error = %v", err)
	}
}

func assertFileContains(t *testing.T, path string, expected string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s error = %v", path, err)
	}
	if !strings.Contains(string(body), expected) {
		t.Fatalf("%s does not contain %q\n%s", path, expected, body)
	}
}

func assertFileNotContains(t *testing.T, path string, unexpected string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s error = %v", path, err)
	}
	if strings.Contains(string(body), unexpected) {
		t.Fatalf("%s unexpectedly contains %q\n%s", path, unexpected, body)
	}
}

func assertGeneratedGoFormatted(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		formatted, err := format.Source(body)
		if err != nil {
			t.Errorf("format generated Go file %s: %v", path, err)
			return nil
		}
		if !bytes.Equal(body, formatted) {
			t.Errorf("generated Go file is not formatted: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk generated project error = %v", err)
	}
}

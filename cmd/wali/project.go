package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	projectFile = ".wali.yaml"
	routesFile  = "api/routes.yaml"
)

var (
	modulePattern = regexp.MustCompile(`^[A-Za-z0-9._~/-]+$`)
	namePattern   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	routePattern  = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)*$`)
)

type Project struct {
	Module string     `yaml:"module"`
	Wali   string     `yaml:"wali"`
	Nodes  []NodeSpec `yaml:"nodes"`
	Routes string     `yaml:"routes"`
}

type NodeSpec struct {
	Name string `yaml:"name"`
}

type RouteManifest struct {
	Routes []RouteSpec `yaml:"routes"`
}

type RouteSpec struct {
	Name  string `yaml:"name"`
	ID    uint32 `yaml:"id"`
	Node  string `yaml:"node"`
	Reply bool   `yaml:"reply"`
}

func newProject(moduleName string, directory string, waliVersion string) (string, error) {
	moduleName = strings.TrimSpace(moduleName)
	if !modulePattern.MatchString(moduleName) ||
		strings.HasPrefix(moduleName, "/") ||
		strings.HasSuffix(moduleName, "/") {
		return "", fmt.Errorf("wali: module无效: %q", moduleName)
	}
	if directory == "" {
		directory = filepath.Base(moduleName)
	}
	root, err := filepath.Abs(directory)
	if err != nil {
		return "", err
	}
	if err := ensureEmptyDirectory(root); err != nil {
		return "", err
	}

	project := Project{
		Module: moduleName,
		Wali:   waliVersion,
		Routes: routesFile,
	}
	if err := writeYAML(filepath.Join(root, projectFile), project, false); err != nil {
		return "", err
	}
	if err := writeYAML(
		filepath.Join(root, routesFile),
		RouteManifest{Routes: []RouteSpec{}},
		false,
	); err != nil {
		return "", err
	}

	files := []templateOutput{
		{Template: "templates/project/go.mod.tmpl", Path: "go.mod"},
		{Template: "templates/project/gitignore.tmpl", Path: ".gitignore"},
		{Template: "templates/project/Makefile.tmpl", Path: "Makefile"},
		{Template: "templates/project/README.md.tmpl", Path: "README.md"},
		{Template: "templates/project/proto_gen.sh.tmpl", Path: "proto_gen.sh"},
		{
			Template: "templates/project/client_api.md.tmpl",
			Path:     "api/client/README.md",
		},
		{
			Template: "templates/project/server_api.md.tmpl",
			Path:     "api/server/README.md",
		},
		{
			Template: "templates/project/infrastructure.go.tmpl",
			Path:     "internal/bootstrap/infrastructure.go",
		},
		{
			Template: "templates/project/auth.go.tmpl",
			Path:     "internal/gateway/auth.go",
		},
		{
			Template: "templates/project/gate.go.tmpl",
			Path:     "cmd/gate/main.go",
		},
		{
			Template: "templates/project/gate.yaml.tmpl",
			Path:     "configs/gate/base.yaml",
		},
	}
	for _, file := range files {
		if err := renderNew(root, file, project); err != nil {
			return "", err
		}
	}
	if err := generateRoutes(root, project, RouteManifest{}); err != nil {
		return "", err
	}
	return root, nil
}

func addNode(root string, name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("wali: Node名称无效: %q", name)
	}
	project, err := loadProject(root)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(project.Nodes, func(node NodeSpec) bool {
		return node.Name == name
	}) {
		return fmt.Errorf("wali: Node已经存在: %s", name)
	}

	project.Nodes = append(project.Nodes, NodeSpec{Name: name})
	if err := writeYAML(filepath.Join(root, projectFile), project, true); err != nil {
		return err
	}
	data := struct {
		Project Project
		Node    NodeSpec
		Service string
	}{
		Project: project,
		Node:    NodeSpec{Name: name},
		Service: goIdentifier(name),
	}
	files := []templateOutput{
		{
			Template: "templates/node/main.go.tmpl",
			Path:     filepath.Join("cmd", name, "main.go"),
		},
		{
			Template: "templates/node/rpc.go.tmpl",
			Path:     filepath.Join("internal", name, "rpc.go"),
		},
		{
			Template: "templates/node/server.proto.tmpl",
			Path: filepath.Join(
				"api",
				"server",
				name,
				"v1",
				name+".proto",
			),
		},
	}
	for _, file := range files {
		if err := renderIfMissing(root, file, data); err != nil {
			return err
		}
	}

	manifest, err := loadRoutes(root, project)
	if err != nil {
		return err
	}
	return generateNodeRoutes(root, project, manifest, name)
}

func addRoute(root string, route RouteSpec) error {
	if !routePattern.MatchString(route.Name) {
		return fmt.Errorf("wali: Route名称无效: %q", route.Name)
	}
	if route.ID == 0 {
		return errors.New("wali: route必须大于0")
	}
	if !namePattern.MatchString(route.Node) {
		return fmt.Errorf("wali: Node名称无效: %q", route.Node)
	}

	project, err := loadProject(root)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(project.Nodes, func(node NodeSpec) bool {
		return node.Name == route.Node
	}) {
		return fmt.Errorf("wali: Node不存在: %s", route.Node)
	}
	manifest, err := loadRoutes(root, project)
	if err != nil {
		return err
	}
	for _, registered := range manifest.Routes {
		if registered.Name == route.Name {
			return fmt.Errorf("wali: Route名称已经存在: %s", route.Name)
		}
		if registered.ID == route.ID {
			return fmt.Errorf(
				"wali: Route ID已经存在: %d (%s)",
				route.ID,
				registered.Name,
			)
		}
	}

	manifest.Routes = append(manifest.Routes, route)
	if err := writeYAML(filepath.Join(root, project.Routes), manifest, true); err != nil {
		return err
	}
	if err := generateRoutes(root, project, manifest); err != nil {
		return err
	}
	if err := generateNodeRoutes(root, project, manifest, route.Node); err != nil {
		return err
	}

	handlerPath := filepath.Join(
		"internal",
		route.Node,
		"handler_"+fileIdentifier(route.Name)+".go",
	)
	return renderIfMissing(
		root,
		templateOutput{
			Template: "templates/route/handler.go.tmpl",
			Path:     handlerPath,
		},
		struct {
			Project    Project
			Route      RouteSpec
			Package    string
			Handler    string
			RouteConst string
		}{
			Project:    project,
			Route:      route,
			Package:    route.Node,
			Handler:    handlerIdentifier(route.Name),
			RouteConst: goIdentifier(route.Name),
		},
	)
}

func findProject() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(current, projectFile)); err == nil {
			return current, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("wali: 当前目录不在Wali项目中")
		}
		current = parent
	}
}

func loadProject(root string) (Project, error) {
	var project Project
	if err := readYAML(filepath.Join(root, projectFile), &project); err != nil {
		return Project{}, err
	}
	if project.Module == "" || project.Routes == "" {
		return Project{}, errors.New("wali: .wali.yaml无效")
	}
	return project, nil
}

func loadRoutes(root string, project Project) (RouteManifest, error) {
	var manifest RouteManifest
	if err := readYAML(filepath.Join(root, project.Routes), &manifest); err != nil {
		return RouteManifest{}, err
	}
	return manifest, nil
}

func readYAML(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		return fmt.Errorf("wali: 解析配置失败 path=%s: %w", path, err)
	}
	return nil
}

func writeYAML(path string, value any, overwrite bool) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	return writeFile(path, data, overwrite)
}

func ensureEmptyDirectory(path string) error {
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(path, 0o755)
	}
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("wali: 目标目录不是空目录: %s", path)
	}
	return nil
}

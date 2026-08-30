package diffgen

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

type protoType struct {
	source sourceFile
	data   dataType
}

type protoGraph struct {
	moduleRoot string
	modulePath string
	types      map[string]protoType
	sources    map[string]sourceFile
}

type protoValue struct {
	name      string
	protoType string
}

type protoList struct {
	name        string
	elementType string
}

func generateProtoFiles(source sourceFile) ([]string, error) {
	graph := newProtoGraph(source.path)
	for _, data := range source.types {
		if err := graph.visit(protoType{source: source, data: data}); err != nil {
			return nil, err
		}
	}

	data, err := graph.renderDataProto(source)
	if err != nil {
		return nil, err
	}
	diff, err := graph.renderDiffProto(source)
	if err != nil {
		return nil, err
	}

	dataPath := strings.TrimSuffix(source.path, ".go") + ".proto"
	diffPath := strings.TrimSuffix(source.path, ".go") + "_diff.proto"
	if err := writeFile(dataPath, data); err != nil {
		return nil, err
	}
	if err := writeFile(diffPath, diff); err != nil {
		return nil, err
	}
	return []string{dataPath, diffPath}, nil
}

func newProtoGraph(path string) *protoGraph {
	moduleRoot, modulePath := findModule(path)
	return &protoGraph{
		moduleRoot: moduleRoot,
		modulePath: modulePath,
		types:      make(map[string]protoType),
		sources:    make(map[string]sourceFile),
	}
}

func (g *protoGraph) visit(value protoType) error {
	key := protoTypeKey(value)
	if _, exists := g.types[key]; exists {
		return nil
	}
	g.types[key] = value

	for _, field := range value.data.fields {
		if field.kind != pointerKind && field.kind != pointerMapKind && field.kind != pointerSliceKind {
			continue
		}
		child, err := g.resolveType(value.source, field.valueType.(*ast.StarExpr).X)
		if err != nil {
			return fmt.Errorf("diffgen: %s.%s: %w", value.data.name, field.name, err)
		}
		if err := g.visit(child); err != nil {
			return err
		}
	}
	return nil
}

func (g *protoGraph) resolveType(source sourceFile, expression ast.Expr) (protoType, error) {
	var packagePath string
	var typeName string
	switch value := expression.(type) {
	case *ast.Ident:
		packagePath = filepath.Dir(source.path)
		typeName = value.Name
	case *ast.SelectorExpr:
		identifier, ok := value.X.(*ast.Ident)
		if !ok {
			return protoType{}, fmt.Errorf("不支持的结构体类型%s", expressionString(source.path, expression))
		}
		importPath := source.imports[identifier.Name]
		if importPath == "" {
			return protoType{}, fmt.Errorf("找不到%s的导入路径", identifier.Name)
		}
		if g.modulePath == "" || importPath != g.modulePath && !strings.HasPrefix(importPath, g.modulePath+"/") {
			return protoType{}, fmt.Errorf("结构体%s不在当前Module中", expressionString(source.path, expression))
		}
		relative := strings.TrimPrefix(strings.TrimPrefix(importPath, g.modulePath), "/")
		packagePath = filepath.Join(g.moduleRoot, filepath.FromSlash(relative))
		typeName = value.Sel.Name
	default:
		return protoType{}, fmt.Errorf("不支持的结构体类型%s", expressionString(source.path, expression))
	}

	entries, err := os.ReadDir(packagePath)
	if err != nil {
		return protoType{}, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_diff.gen.go") {
			continue
		}
		path := filepath.Join(packagePath, name)
		parsed, exists := g.sources[path]
		if !exists {
			parsed, err = parseSource(path)
			if err != nil {
				return protoType{}, err
			}
			g.sources[path] = parsed
		}
		if !parsed.schema {
			continue
		}
		for _, data := range parsed.types {
			if data.name == typeName {
				return protoType{source: parsed, data: data}, nil
			}
		}
	}
	return protoType{}, fmt.Errorf("找不到diff数据类型%s", typeName)
}

func (g *protoGraph) renderDataProto(source sourceFile) ([]byte, error) {
	imports := make(map[string]struct{})
	for _, data := range source.types {
		for _, field := range data.fields {
			if field.kind != pointerKind && field.kind != pointerMapKind && field.kind != pointerSliceKind {
				continue
			}
			child, err := g.resolveType(source, field.valueType.(*ast.StarExpr).X)
			if err != nil {
				return nil, err
			}
			if child.source.path != source.path {
				imports[g.protoFileImport(child.source)] = struct{}{}
			}
		}
	}

	var output strings.Builder
	writeProtoHeader(&output, protoPackage(source), g.goPackage(source), csharpNamespace(source))
	writeProtoImports(&output, imports)
	for _, data := range source.types {
		fmt.Fprintf(&output, "message %s {\n", data.name)
		for _, field := range data.fields {
			fieldType, err := g.protoFieldType(source, field)
			if err != nil {
				return nil, fmt.Errorf("diffgen: %s.%s: %w", data.name, field.name, err)
			}
			fmt.Fprintf(&output, "  %s %s = %d;\n", fieldType, snakeCase(field.name), field.diffIndex)
		}
		output.WriteString("}\n\n")
	}
	return []byte(output.String()), nil
}

func (g *protoGraph) renderDiffProto(source sourceFile) ([]byte, error) {
	imports := map[string]struct{}{g.protoFileImport(source): {}}
	values := make(map[string]protoValue)
	lists := make(map[string]protoList)

	types := make([]protoType, 0, len(g.types))
	for _, value := range g.types {
		types = append(types, value)
	}
	sort.Slice(types, func(left int, right int) bool {
		return protoTypeKey(types[left]) < protoTypeKey(types[right])
	})
	typeNameCounts := make(map[string]int, len(types))
	for _, value := range types {
		typeNameCounts[value.data.name]++
	}

	for _, value := range types {
		imports[g.protoFileImport(value.source)] = struct{}{}
		messageType := absoluteProtoType(value)
		name := diffProtoTypeName(value, typeNameCounts)
		values["message:"+protoTypeKey(value)] = protoValue{name: snakeCase(name) + "_value", protoType: messageType}
		for _, field := range value.data.fields {
			switch field.kind {
			case primitiveKind, primitiveMapKind:
				protoType, err := protoScalar(field.valueType)
				if err != nil {
					return nil, err
				}
				values["scalar:"+protoType] = protoValue{name: protoValueName(protoType), protoType: protoType}
			case primitiveSliceKind:
				protoType, err := protoScalar(field.valueType)
				if err != nil {
					return nil, err
				}
				if protoType == "bytes" {
					values["scalar:bytes"] = protoValue{name: "bytes_value", protoType: "bytes"}
					continue
				}
				name := upperCamel(protoType) + "List"
				lists["list:"+protoType] = protoList{name: name, elementType: protoType}
				values["list:"+protoType] = protoValue{name: snakeCase(name) + "_value", protoType: name}
			case pointerSliceKind:
				child, err := g.resolveType(value.source, field.valueType.(*ast.StarExpr).X)
				if err != nil {
					return nil, err
				}
				name := diffProtoTypeName(child, typeNameCounts) + "List"
				lists["list:"+protoTypeKey(child)] = protoList{name: name, elementType: absoluteProtoType(child)}
				values["list:"+protoTypeKey(child)] = protoValue{name: snakeCase(name) + "_value", protoType: name}
			}
		}
	}

	var output strings.Builder
	diffPackage := protoPackage(source) + ".diff"
	writeProtoHeader(&output, diffPackage, g.diffGoPackage(source), csharpNamespace(source)+".Diff")
	writeProtoImports(&output, imports)
	output.WriteString("enum Operation {\n")
	output.WriteString("  OPERATION_UNKNOWN = 0;\n")
	output.WriteString("  PRIMITIVE_SET = 1;\n")
	output.WriteString("  POINTER_SET = 2;\n")
	output.WriteString("  POINTER_CLEAR = 3;\n")
	output.WriteString("  MAP_SET = 4;\n")
	output.WriteString("  MAP_DELETE = 5;\n")
	output.WriteString("  MAP_CLEAR = 6;\n")
	output.WriteString("  SLICE_REPLACE = 7;\n")
	output.WriteString("}\n\n")
	output.WriteString("message PathNode {\n")
	output.WriteString("  uint32 field_number = 1;\n\n")
	output.WriteString("  oneof map_key {\n")
	output.WriteString("    bool bool_key = 10;\n")
	output.WriteString("    int32 int32_key = 11;\n")
	output.WriteString("    int64 int64_key = 12;\n")
	output.WriteString("    uint32 uint32_key = 13;\n")
	output.WriteString("    uint64 uint64_key = 14;\n")
	output.WriteString("    string string_key = 15;\n")
	output.WriteString("  }\n")
	output.WriteString("}\n\n")

	listKeys := sortedProtoListKeys(lists)
	for _, key := range listKeys {
		list := lists[key]
		fmt.Fprintf(&output, "message %s {\n  repeated %s values = 1;\n}\n\n", list.name, list.elementType)
	}

	output.WriteString("message Patch {\n")
	output.WriteString("  repeated PathNode path = 1;\n")
	output.WriteString("  Operation operation = 2;\n\n")
	output.WriteString("  oneof value {\n")
	valueKeys := sortedProtoValueKeys(values)
	for index, key := range valueKeys {
		value := values[key]
		fmt.Fprintf(&output, "    %s %s = %d;\n", value.protoType, value.name, 10+index)
	}
	output.WriteString("  }\n")
	output.WriteString("}\n\n")
	output.WriteString("message Delta {\n")
	output.WriteString("  uint32 schema_version = 1;\n")
	output.WriteString("  repeated Patch patches = 2;\n")
	output.WriteString("}\n\n")

	for _, data := range source.types {
		fmt.Fprintf(&output, "message %sSyncPush {\n", data.name)
		output.WriteString("  uint32 schema_version = 1;\n")
		output.WriteString("  uint64 base_version = 2;\n")
		output.WriteString("  uint64 version = 3;\n\n")
		output.WriteString("  oneof payload {\n")
		fmt.Fprintf(&output, "    .%s.%s full = 10;\n", protoPackage(source), data.name)
		output.WriteString("    Delta delta = 11;\n")
		output.WriteString("  }\n")
		output.WriteString("}\n\n")
	}
	return []byte(output.String()), nil
}

func (g *protoGraph) protoFieldType(source sourceFile, field dataField) (string, error) {
	switch field.kind {
	case primitiveKind:
		return protoScalar(field.valueType)
	case pointerKind:
		value, err := g.resolveType(source, field.valueType.(*ast.StarExpr).X)
		if err != nil {
			return "", err
		}
		return qualifiedProtoType(source, value), nil
	case primitiveMapKind:
		key, err := protoScalar(field.keyType)
		if err != nil {
			return "", err
		}
		value, err := protoScalar(field.valueType)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("map<%s, %s>", key, value), nil
	case pointerMapKind:
		key, err := protoScalar(field.keyType)
		if err != nil {
			return "", err
		}
		value, err := g.resolveType(source, field.valueType.(*ast.StarExpr).X)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("map<%s, %s>", key, qualifiedProtoType(source, value)), nil
	case primitiveSliceKind:
		value, err := protoScalar(field.valueType)
		if err != nil {
			return "", err
		}
		if value == "bytes" {
			return "bytes", nil
		}
		return "repeated " + value, nil
	case pointerSliceKind:
		value, err := g.resolveType(source, field.valueType.(*ast.StarExpr).X)
		if err != nil {
			return "", err
		}
		return "repeated " + qualifiedProtoType(source, value), nil
	default:
		return "", fmt.Errorf("未知字段类型")
	}
}

func protoScalar(expression ast.Expr) (string, error) {
	if slice, ok := expression.(*ast.ArrayType); ok && slice.Len == nil {
		identifier, ok := slice.Elt.(*ast.Ident)
		if ok && identifier.Name == "byte" {
			return "bytes", nil
		}
	}
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return "", fmt.Errorf("%s不是Proto基础类型", expressionString("schema", expression))
	}
	switch identifier.Name {
	case "bool", "int32", "int64", "uint32", "uint64", "string":
		return identifier.Name, nil
	case "float32":
		return "float", nil
	case "float64":
		return "double", nil
	case "byte":
		return "bytes", nil
	default:
		return "", fmt.Errorf("%s不是Proto基础类型", identifier.Name)
	}
}

func protoTypeKey(value protoType) string {
	return filepath.Clean(value.source.path) + "#" + value.data.name
}

func qualifiedProtoType(root sourceFile, value protoType) string {
	if protoPackage(root) == protoPackage(value.source) {
		return value.data.name
	}
	return "." + protoPackage(value.source) + "." + value.data.name
}

func absoluteProtoType(value protoType) string {
	return "." + protoPackage(value.source) + "." + value.data.name
}

func diffProtoTypeName(value protoType, counts map[string]int) string {
	if counts[value.data.name] == 1 {
		return value.data.name
	}
	return upperCamel(value.source.packageName) + value.data.name
}

func protoValueName(protoType string) string {
	return snakeCase(protoType) + "_value"
}

func sortedProtoValueKeys(values map[string]protoValue) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedProtoListKeys(values map[string]protoList) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func writeProtoHeader(output *strings.Builder, packageName string, goPackage string, csharp string) {
	output.WriteString("// Code generated by diff-gen. DO NOT EDIT.\n\n")
	output.WriteString("syntax = \"proto3\";\n\n")
	fmt.Fprintf(output, "package %s;\n\n", packageName)
	fmt.Fprintf(output, "option go_package = \"%s\";\n", goPackage)
	fmt.Fprintf(output, "option csharp_namespace = \"%s\";\n\n", csharp)
}

func writeProtoImports(output *strings.Builder, imports map[string]struct{}) {
	paths := make([]string, 0, len(imports))
	for path := range imports {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		fmt.Fprintf(output, "import \"%s\";\n", path)
	}
	if len(paths) != 0 {
		output.WriteByte('\n')
	}
}

func (g *protoGraph) protoFileImport(source sourceFile) string {
	protoPath := strings.TrimSuffix(source.path, ".go") + ".proto"
	if g.moduleRoot == "" {
		return filepath.Base(protoPath)
	}
	relative, err := filepath.Rel(g.moduleRoot, protoPath)
	if err != nil {
		return filepath.Base(protoPath)
	}
	return filepath.ToSlash(relative)
}

func (g *protoGraph) goPackage(source sourceFile) string {
	if g.moduleRoot == "" || g.modulePath == "" {
		return "generated/" + source.packageName + "/pb;pb" + upperCamel(source.packageName)
	}
	relative, err := filepath.Rel(g.moduleRoot, filepath.Dir(source.path))
	if err != nil || relative == "." {
		return g.modulePath + "/pb;pb" + upperCamel(source.packageName)
	}
	return g.modulePath + "/" + filepath.ToSlash(relative) + "/pb;pb" + upperCamel(source.packageName)
}

func (g *protoGraph) diffGoPackage(source sourceFile) string {
	value := g.goPackage(source)
	separator := strings.IndexByte(value, ';')
	if separator == -1 {
		return value + "/diff"
	}
	return value[:separator] + "/diff;" + value[separator+1:] + "Diff"
}

func protoPackage(source sourceFile) string {
	return source.packageName
}

func csharpNamespace(source sourceFile) string {
	return "Nova.Generated." + upperCamel(source.packageName)
}

func findModule(path string) (string, string) {
	directory := filepath.Dir(path)
	for {
		goModPath := filepath.Join(directory, "go.mod")
		data, err := os.ReadFile(goModPath)
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					return directory, strings.TrimSpace(strings.TrimPrefix(line, "module "))
				}
			}
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", ""
		}
		directory = parent
	}
}

func snakeCase(value string) string {
	var output strings.Builder
	runes := []rune(value)
	for index, current := range runes {
		if unicode.IsUpper(current) {
			if index != 0 && (unicode.IsLower(runes[index-1]) || unicode.IsDigit(runes[index-1]) || index+1 < len(runes) && unicode.IsLower(runes[index+1])) {
				output.WriteByte('_')
			}
			output.WriteRune(unicode.ToLower(current))
			continue
		}
		output.WriteRune(current)
	}
	return output.String()
}

func upperCamel(value string) string {
	var output strings.Builder
	upper := true
	for _, current := range value {
		if current == '_' || current == '-' || current == '.' {
			upper = true
			continue
		}
		if upper {
			output.WriteRune(unicode.ToUpper(current))
			upper = false
			continue
		}
		output.WriteRune(current)
	}
	return output.String()
}

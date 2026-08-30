package diffgen

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const diffPackagePath = "github.com/2comjie/nova/diff"

type wrapperKind uint8

const (
	primitiveKind      wrapperKind = 1
	pointerKind        wrapperKind = 2
	primitiveMapKind   wrapperKind = 3
	pointerMapKind     wrapperKind = 4
	primitiveSliceKind wrapperKind = 5
	pointerSliceKind   wrapperKind = 6
)

type sourceFile struct {
	path        string
	packageName string
	imports     map[string]string
	types       []dataType
	schema      bool
}

type dataType struct {
	name           string
	typeParams     []string
	typeParamsNode *ast.FieldList
	runtimeFields  []*ast.Field
	fields         []dataField
}

type dataField struct {
	name        string
	runtimeName string
	diffIndex   uint32
	kind        wrapperKind
	keyType     ast.Expr
	valueType   ast.Expr
}

func Generate(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	generatedFiles := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_diff.gen.go") {
			continue
		}

		path := filepath.Join(dir, name)
		source, err := parseSource(path)
		if err != nil {
			return nil, err
		}
		if len(source.types) == 0 {
			continue
		}

		data, err := renderSource(source)
		if err != nil {
			return nil, err
		}
		output := strings.TrimSuffix(path, ".go") + "_diff.gen.go"
		if err := writeFile(output, data); err != nil {
			return nil, err
		}
		generatedFiles = append(generatedFiles, output)

		if source.schema {
			protoFiles, err := generateProtoFiles(source)
			if err != nil {
				return nil, err
			}
			generatedFiles = append(generatedFiles, protoFiles...)
		}
	}
	return generatedFiles, nil
}

func parseSource(path string) (sourceFile, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
	if err != nil {
		return sourceFile{}, err
	}

	source := sourceFile{
		path:        path,
		packageName: file.Name.Name,
		imports:     make(map[string]string, len(file.Imports)),
		schema:      hasDiffFastBuildTag(file),
	}
	for _, importSpec := range file.Imports {
		importPath, err := strconv.Unquote(importSpec.Path.Value)
		if err != nil {
			return sourceFile{}, fmt.Errorf("diffgen: %s: %w", path, err)
		}

		alias := filepath.Base(importPath)
		if importSpec.Name != nil {
			alias = importSpec.Name.Name
		}
		source.imports[alias] = importPath
	}

	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			typeSpec := specification.(*ast.TypeSpec)
			structure, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			data, found, err := parseDataType(source, typeSpec, structure)
			if err != nil {
				return sourceFile{}, err
			}
			if found {
				source.types = append(source.types, data)
			}
		}
	}
	return source, nil
}

func parseDataType(source sourceFile, typeSpec *ast.TypeSpec, structure *ast.StructType) (dataType, bool, error) {
	data := dataType{name: typeSpec.Name.Name, typeParamsNode: typeSpec.TypeParams}
	if typeSpec.TypeParams != nil {
		for _, field := range typeSpec.TypeParams.List {
			for _, name := range field.Names {
				data.typeParams = append(data.typeParams, name.Name)
			}
		}
	}

	if source.schema {
		if typeSpec.TypeParams != nil {
			return dataType{}, false, fmt.Errorf("diffgen: %s: diff数据类型不支持泛型，请使用具体类型和组合", data.name)
		}
		return parseSchemaType(source, data, structure)
	}

	hasObject := false
	hasWrapper := false
	indexes := make(map[uint32]string)
	for _, field := range structure.Fields.List {
		if len(field.Names) == 0 {
			if isDiffSelector(field.Type, source.imports, "Object") {
				hasObject = true
			}
			continue
		}

		kind, arguments, wrapper := parseWrapper(field.Type, source.imports)
		if !wrapper {
			if _, tagged := diffTag(field); tagged {
				return dataType{}, false, fmt.Errorf("diffgen: %s.%s: diff字段必须使用diff包装类型", data.name, field.Names[0].Name)
			}
			continue
		}
		hasWrapper = true
		if len(field.Names) != 1 {
			return dataType{}, false, fmt.Errorf("diffgen: %s: 一个diff字段只能声明一个名字", data.name)
		}

		tag, tagged := diffTag(field)
		if !tagged {
			return dataType{}, false, fmt.Errorf("diffgen: %s.%s: 缺少diff标签", data.name, field.Names[0].Name)
		}
		index, err := strconv.ParseUint(tag, 10, 32)
		if err != nil || index == 0 {
			return dataType{}, false, fmt.Errorf("diffgen: %s.%s: diff标签必须是大于0的uint32", data.name, field.Names[0].Name)
		}
		if previous := indexes[uint32(index)]; previous != "" {
			return dataType{}, false, fmt.Errorf("diffgen: %s.%s和%s使用了相同的diff标签%d", data.name, previous, field.Names[0].Name, index)
		}
		indexes[uint32(index)] = field.Names[0].Name

		parsedField := dataField{
			name:        field.Names[0].Name,
			runtimeName: field.Names[0].Name,
			diffIndex:   uint32(index),
			kind:        kind,
		}
		switch kind {
		case primitiveKind, pointerKind, primitiveSliceKind, pointerSliceKind:
			parsedField.valueType = arguments[0]
		case primitiveMapKind, pointerMapKind:
			parsedField.keyType = arguments[0]
			parsedField.valueType = arguments[1]
		}
		if kind == pointerKind || kind == pointerMapKind || kind == pointerSliceKind {
			if _, ok := parsedField.valueType.(*ast.StarExpr); !ok {
				return dataType{}, false, fmt.Errorf("diffgen: %s.%s: Pointer值必须是指针类型", data.name, parsedField.name)
			}
		}
		data.fields = append(data.fields, parsedField)
	}

	if !hasObject {
		if hasWrapper {
			return dataType{}, false, fmt.Errorf("diffgen: %s: 必须匿名嵌入diff.Object", data.name)
		}
		return dataType{}, false, nil
	}
	if typeSpec.TypeParams != nil {
		return dataType{}, false, fmt.Errorf("diffgen: %s: diff数据类型不支持泛型，请使用具体类型和组合", data.name)
	}
	sort.Slice(data.fields, func(left int, right int) bool {
		return data.fields[left].diffIndex < data.fields[right].diffIndex
	})
	return data, true, nil
}

func parseSchemaType(source sourceFile, data dataType, structure *ast.StructType) (dataType, bool, error) {
	indexes := make(map[uint32]string)
	for _, field := range structure.Fields.List {
		tag, tagged := diffTag(field)
		if tag == "-" || !tagged && len(field.Names) == 0 {
			data.runtimeFields = append(data.runtimeFields, field)
			continue
		}
		if !tagged {
			return dataType{}, false, fmt.Errorf("diffgen: %s.%s: 缺少diff标签", data.name, field.Names[0].Name)
		}
		if len(field.Names) == 0 {
			return dataType{}, false, fmt.Errorf("diffgen: %s: 匿名字段不能参与diff", data.name)
		}
		if len(field.Names) != 1 {
			return dataType{}, false, fmt.Errorf("diffgen: %s: 一个diff字段只能声明一个名字", data.name)
		}
		index, err := strconv.ParseUint(tag, 10, 32)
		if err != nil || index == 0 {
			return dataType{}, false, fmt.Errorf("diffgen: %s.%s: diff标签必须是大于0的uint32", data.name, field.Names[0].Name)
		}
		if previous := indexes[uint32(index)]; previous != "" {
			return dataType{}, false, fmt.Errorf("diffgen: %s.%s和%s使用了相同的diff标签%d", data.name, previous, field.Names[0].Name, index)
		}
		indexes[uint32(index)] = field.Names[0].Name

		kind, keyType, valueType, err := classifySchemaType(field.Type)
		if err != nil {
			return dataType{}, false, fmt.Errorf("diffgen: %s.%s: %w", data.name, field.Names[0].Name, err)
		}
		data.fields = append(data.fields, dataField{
			name:        field.Names[0].Name,
			runtimeName: lowerFirst(field.Names[0].Name),
			diffIndex:   uint32(index),
			kind:        kind,
			keyType:     keyType,
			valueType:   valueType,
		})
	}
	if len(data.fields) == 0 {
		return dataType{}, false, nil
	}
	sort.Slice(data.fields, func(left int, right int) bool {
		return data.fields[left].diffIndex < data.fields[right].diffIndex
	})
	return data, true, nil
}

func classifySchemaType(expression ast.Expr) (wrapperKind, ast.Expr, ast.Expr, error) {
	if containsGenericType(expression) {
		return 0, nil, nil, errors.New("diff字段不支持泛型实例，请使用具体类型和组合")
	}
	switch value := expression.(type) {
	case *ast.StarExpr:
		if !isNamedType(value.X) {
			return 0, nil, nil, errors.New("Pointer只支持命名结构体指针")
		}
		return pointerKind, nil, expression, nil
	case *ast.MapType:
		if !isProtoMapKey(value.Key) {
			return 0, nil, nil, errors.New("Map的Key必须是bool、int32、int64、uint32、uint64或string")
		}
		if pointer, ok := value.Value.(*ast.StarExpr); ok {
			if !isNamedType(pointer.X) {
				return 0, nil, nil, errors.New("PointerMap只支持命名结构体指针")
			}
			return pointerMapKind, value.Key, value.Value, nil
		}
		if !isPrimitiveType(value.Value) {
			return 0, nil, nil, errors.New("Map的Value只能是基础类型或结构体指针")
		}
		return primitiveMapKind, value.Key, value.Value, nil
	case *ast.ArrayType:
		if value.Len != nil {
			return 0, nil, nil, errors.New("不支持定长数组")
		}
		if identifier, ok := value.Elt.(*ast.Ident); ok && identifier.Name == "byte" {
			return primitiveSliceKind, nil, value.Elt, nil
		}
		if pointer, ok := value.Elt.(*ast.StarExpr); ok {
			if !isNamedType(pointer.X) {
				return 0, nil, nil, errors.New("PointerSlice只支持命名结构体指针")
			}
			return pointerSliceKind, nil, value.Elt, nil
		}
		if !isPrimitiveType(value.Elt) {
			return 0, nil, nil, errors.New("Slice的元素只能是基础类型或结构体指针")
		}
		return primitiveSliceKind, nil, value.Elt, nil
	default:
		if !isPrimitiveType(expression) {
			return 0, nil, nil, errors.New("字段只能是基础类型、结构体指针、Map或Slice")
		}
		return primitiveKind, nil, expression, nil
	}
}

func isPrimitiveType(expression ast.Expr) bool {
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return false
	}
	switch identifier.Name {
	case "bool", "int32", "int64", "uint32", "uint64", "float32", "float64", "string":
		return true
	default:
		return false
	}
}

func isNamedType(expression ast.Expr) bool {
	switch expression.(type) {
	case *ast.Ident, *ast.SelectorExpr:
		return true
	default:
		return false
	}
}

func isProtoMapKey(expression ast.Expr) bool {
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return false
	}
	switch identifier.Name {
	case "bool", "int32", "int64", "uint32", "uint64", "string":
		return true
	default:
		return false
	}
}

func containsGenericType(expression ast.Expr) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		switch node.(type) {
		case *ast.IndexExpr, *ast.IndexListExpr:
			found = true
			return false
		default:
			return true
		}
	})
	return found
}

func hasDiffFastBuildTag(file *ast.File) bool {
	for _, commentGroup := range file.Comments {
		for _, comment := range commentGroup.List {
			if strings.TrimSpace(comment.Text) == "//go:build diff_fast" {
				return true
			}
		}
	}
	return false
}

func parseWrapper(expression ast.Expr, imports map[string]string) (wrapperKind, []ast.Expr, bool) {
	var base ast.Expr
	var arguments []ast.Expr
	switch generic := expression.(type) {
	case *ast.IndexExpr:
		base = generic.X
		arguments = []ast.Expr{generic.Index}
	case *ast.IndexListExpr:
		base = generic.X
		arguments = generic.Indices
	default:
		return 0, nil, false
	}

	selector, ok := base.(*ast.SelectorExpr)
	if !ok {
		return 0, nil, false
	}
	packageName, ok := selector.X.(*ast.Ident)
	if !ok || imports[packageName.Name] != diffPackagePath {
		return 0, nil, false
	}

	var kind wrapperKind
	var argumentCount int
	switch selector.Sel.Name {
	case "Primitive":
		kind, argumentCount = primitiveKind, 1
	case "Pointer":
		kind, argumentCount = pointerKind, 1
	case "PrimitiveMap":
		kind, argumentCount = primitiveMapKind, 2
	case "PointerMap":
		kind, argumentCount = pointerMapKind, 2
	case "PrimitiveSlice":
		kind, argumentCount = primitiveSliceKind, 1
	case "PointerSlice":
		kind, argumentCount = pointerSliceKind, 1
	default:
		return 0, nil, false
	}
	if len(arguments) != argumentCount {
		return 0, nil, false
	}
	return kind, arguments, true
}

func isDiffSelector(expression ast.Expr, imports map[string]string, name string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	return ok && imports[packageName.Name] == diffPackagePath
}

func diffTag(field *ast.Field) (string, bool) {
	if field.Tag == nil {
		return "", false
	}
	tag, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		return "", false
	}
	return reflect.StructTag(tag).Lookup("diff")
}

func runtimeFieldType(source sourceFile, field dataField, diffAlias string) string {
	valueType := expressionString(source.path, field.valueType)
	switch field.kind {
	case primitiveKind:
		return fmt.Sprintf("%s.Primitive[%s]", diffAlias, valueType)
	case pointerKind:
		return fmt.Sprintf("%s.Pointer[%s]", diffAlias, valueType)
	case primitiveMapKind:
		return fmt.Sprintf("%s.PrimitiveMap[%s, %s]", diffAlias, expressionString(source.path, field.keyType), valueType)
	case pointerMapKind:
		return fmt.Sprintf("%s.PointerMap[%s, %s]", diffAlias, expressionString(source.path, field.keyType), valueType)
	case primitiveSliceKind:
		return fmt.Sprintf("%s.PrimitiveSlice[%s]", diffAlias, valueType)
	case pointerSliceKind:
		return fmt.Sprintf("%s.PointerSlice[%s]", diffAlias, valueType)
	default:
		panic("diffgen: 未知字段类型")
	}
}

func pointerElementType(source sourceFile, expression ast.Expr) string {
	return expressionString(source.path, expression.(*ast.StarExpr).X)
}

func receiverType(data dataType) string {
	name := data.name
	if len(data.typeParams) != 0 {
		name += "[" + strings.Join(data.typeParams, ", ") + "]"
	}
	return name
}

func lowerFirst(value string) string {
	if value[0] < 'A' || value[0] > 'Z' {
		return value
	}
	return string(value[0]+'a'-'A') + value[1:]
}

func requiredImports(source sourceFile) map[string]string {
	imports := make(map[string]string)
	for _, data := range source.types {
		if data.typeParamsNode != nil {
			collectExpressionImports(data.typeParamsNode, source.imports, imports)
		}
		for _, field := range data.fields {
			collectExpressionImports(field.keyType, source.imports, imports)
			collectExpressionImports(field.valueType, source.imports, imports)
		}
		for _, field := range data.runtimeFields {
			collectExpressionImports(field.Type, source.imports, imports)
		}
	}
	return imports
}

func collectExpressionImports(expression ast.Node, available map[string]string, imports map[string]string) {
	if expression == nil {
		return
	}
	ast.Inspect(expression, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		if importPath := available[identifier.Name]; importPath != "" {
			imports[identifier.Name] = importPath
		}
		return true
	})
}

func generatedDiffAlias(source sourceFile) string {
	required := requiredImports(source)
	for index := 0; ; index++ {
		alias := "diff"
		if index != 0 {
			alias += strconv.Itoa(index)
		}
		if importPath := required[alias]; importPath == "" || importPath == diffPackagePath {
			return alias
		}
	}
}

func expressionString(path string, expression ast.Node) string {
	var data bytes.Buffer
	if err := format.Node(&data, token.NewFileSet(), expression); err != nil {
		panic(fmt.Sprintf("diffgen: %s: %v", path, err))
	}
	return data.String()
}

func fieldString(path string, field *ast.Field) string {
	declaration := expressionString(path, field.Type)
	if len(field.Names) != 0 {
		names := make([]string, len(field.Names))
		for index, name := range field.Names {
			names[index] = name.Name
		}
		declaration = strings.Join(names, ", ") + " " + declaration
	}
	if field.Tag != nil {
		declaration += " " + field.Tag.Value
	}
	return declaration
}

func typeParamsDeclaration(path string, fields *ast.FieldList) string {
	parameters := make([]string, 0, len(fields.List))
	for _, field := range fields.List {
		names := make([]string, 0, len(field.Names))
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
		parameters = append(parameters, strings.Join(names, ", ")+" "+expressionString(path, field.Type))
	}
	return "[" + strings.Join(parameters, ", ") + "]"
}

func pathRootParameter(data dataType) string {
	root := "DiffRoot"
	for {
		used := false
		for _, parameter := range data.typeParams {
			if parameter == root {
				used = true
				break
			}
		}
		if !used {
			return root
		}
		root += "Type"
	}
}

func pathTypeParameters(path string, data dataType, root string) string {
	parameters := root + " any"
	if data.typeParamsNode != nil {
		parameters += ", " + strings.TrimSuffix(strings.TrimPrefix(typeParamsDeclaration(path, data.typeParamsNode), "["), "]")
	}
	return "[" + parameters + "]"
}

func pathTypeArguments(data dataType, root string) string {
	arguments := append([]string{root}, data.typeParams...)
	return "[" + strings.Join(arguments, ", ") + "]"
}

func pathRootTypeArguments(data dataType) string {
	arguments := []string{"*" + receiverType(data)}
	arguments = append(arguments, data.typeParams...)
	return "[" + strings.Join(arguments, ", ") + "]"
}

func pointerPathType(source sourceFile, expression ast.Expr, root string) (string, string) {
	pointer := expression.(*ast.StarExpr)
	qualifier, name, arguments := namedTypeParts(source.path, pointer.X)
	arguments = append([]string{root}, arguments...)
	typeName := qualifier + name + "DiffPath[" + strings.Join(arguments, ", ") + "]"
	constructor := qualifier + "New" + name + "DiffPath[" + strings.Join(arguments, ", ") + "]"
	return typeName, constructor
}

func namedTypeParts(path string, expression ast.Expr) (string, string, []string) {
	switch value := expression.(type) {
	case *ast.Ident:
		return "", value.Name, nil
	case *ast.SelectorExpr:
		return expressionString(path, value.X) + ".", value.Sel.Name, nil
	case *ast.IndexExpr:
		qualifier, name, arguments := namedTypeParts(path, value.X)
		return qualifier, name, append(arguments, expressionString(path, value.Index))
	case *ast.IndexListExpr:
		qualifier, name, arguments := namedTypeParts(path, value.X)
		for _, index := range value.Indices {
			arguments = append(arguments, expressionString(path, index))
		}
		return qualifier, name, arguments
	default:
		panic("diffgen: 非法的Pointer类型")
	}
}

func writeFile(path string, data []byte) error {
	oldData, err := os.ReadFile(path)
	if err == nil && bytes.Equal(oldData, data) {
		return nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	temporary, err := os.CreateTemp(filepath.Dir(path), ".diff-gen-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

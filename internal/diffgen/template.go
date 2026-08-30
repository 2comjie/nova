package diffgen

import (
	"bytes"
	"embed"
	"fmt"
	"go/format"
	"sort"
	"text/template"
)

//go:embed templates/*.tpl
var templateFiles embed.FS

var codeTemplates = template.Must(template.New("diffgen").ParseFS(templateFiles, "templates/*.tpl"))

type templateFile struct {
	Schema      bool
	PackageName string
	DiffAlias   string
	Imports     []templateImport
	Types       []templateType
}

type templateImport struct {
	Alias string
	Path  string
}

type templateType struct {
	Name             string
	TypeParams       string
	Receiver         string
	DiffAlias        string
	PathRoot         string
	PathTypeParams   string
	PathTypeArgs     string
	PathRootTypeArgs string
	Generic          bool
	Fields           []templateField
}

type templateField struct {
	Name            string
	RuntimeName     string
	DiffIndex       uint32
	DiffAlias       string
	KeyType         string
	ValueType       string
	ElementType     string
	RuntimeType     string
	Primitive       bool
	Pointer         bool
	PrimitiveMap    bool
	PointerMap      bool
	PrimitiveSlice  bool
	PointerSlice    bool
	Map             bool
	Slice           bool
	Collection      bool
	PathType        string
	PathConstructor string
	PathContainer   string
}

func renderSource(source sourceFile) ([]byte, error) {
	model := buildTemplateFile(source)
	var body bytes.Buffer
	if err := codeTemplates.ExecuteTemplate(&body, "file.go.tpl", model); err != nil {
		return nil, err
	}
	formatted, err := format.Source(body.Bytes())
	if err != nil {
		return nil, fmt.Errorf("diffgen: 生成%s失败: %w\n%s", source.path, err, body.String())
	}
	return formatted, nil
}

func buildTemplateFile(source sourceFile) templateFile {
	diffAlias := generatedDiffAlias(source)
	model := templateFile{
		Schema:      source.schema,
		PackageName: source.packageName,
		DiffAlias:   diffAlias,
		Types:       make([]templateType, 0, len(source.types)),
	}
	for alias, importPath := range requiredImports(source) {
		if importPath != diffPackagePath {
			model.Imports = append(model.Imports, templateImport{Alias: alias, Path: importPath})
		}
	}
	sort.Slice(model.Imports, func(left int, right int) bool {
		return model.Imports[left].Alias < model.Imports[right].Alias
	})

	for _, data := range source.types {
		pathRoot := pathRootParameter(data)
		typeModel := templateType{
			Name:             data.name,
			Receiver:         receiverType(data),
			DiffAlias:        diffAlias,
			PathRoot:         pathRoot,
			PathTypeParams:   pathTypeParameters(source.path, data, pathRoot),
			PathTypeArgs:     pathTypeArguments(data, pathRoot),
			PathRootTypeArgs: pathRootTypeArguments(data),
			Generic:          len(data.typeParams) != 0,
			Fields:           make([]templateField, 0, len(data.fields)),
		}
		if data.typeParamsNode != nil {
			typeModel.TypeParams = typeParamsDeclaration(source.path, data.typeParamsNode)
		}
		for _, field := range data.fields {
			fieldModel := templateField{
				Name:          field.name,
				RuntimeName:   field.runtimeName,
				DiffIndex:     field.diffIndex,
				DiffAlias:     diffAlias,
				ValueType:     expressionString(source.path, field.valueType),
				PathContainer: data.name + field.name + "DiffPath",
			}
			if field.keyType != nil {
				fieldModel.KeyType = expressionString(source.path, field.keyType)
			}
			switch field.kind {
			case primitiveKind:
				fieldModel.Primitive = true
			case pointerKind:
				fieldModel.Pointer = true
				fieldModel.ElementType = pointerElementType(source, field.valueType)
				fieldModel.PathType, fieldModel.PathConstructor = pointerPathType(source, field.valueType, pathRoot)
			case primitiveMapKind:
				fieldModel.PrimitiveMap = true
				fieldModel.Map = true
				fieldModel.Collection = true
			case pointerMapKind:
				fieldModel.PointerMap = true
				fieldModel.Map = true
				fieldModel.Collection = true
				fieldModel.ElementType = pointerElementType(source, field.valueType)
				fieldModel.PathType, fieldModel.PathConstructor = pointerPathType(source, field.valueType, pathRoot)
			case primitiveSliceKind:
				fieldModel.PrimitiveSlice = true
				fieldModel.Slice = true
				fieldModel.Collection = true
			case pointerSliceKind:
				fieldModel.PointerSlice = true
				fieldModel.Slice = true
				fieldModel.Collection = true
				fieldModel.ElementType = pointerElementType(source, field.valueType)
				fieldModel.PathType, fieldModel.PathConstructor = pointerPathType(source, field.valueType, pathRoot)
			}
			fieldModel.RuntimeType = runtimeFieldType(source, field, diffAlias)
			typeModel.Fields = append(typeModel.Fields, fieldModel)
		}
		model.Types = append(model.Types, typeModel)
	}
	return model
}

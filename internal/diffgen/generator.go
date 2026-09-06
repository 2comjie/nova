package diffgen

import (
	"bytes"
	"embed"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"text/template"

	"github.com/spf13/cast"
	"github.com/stoewer/go-strcase"
	"golang.org/x/tools/go/packages"
)

const diffPackagePath = "github.com/2comjie/nova/diff"

//go:embed templates/*.tpl
var templateFile embed.FS

var codeTemplate = template.Must(
	template.ParseFS(templateFile, "templates/*.tpl"),
)

type fieldKind string

const (
	primitiveKind      fieldKind = "primitive"
	pointerKind        fieldKind = "pointer"
	primitiveMapKind   fieldKind = "primitiveMap"
	pointerMapKind     fieldKind = "pointerMap"
	primitiveSliceKind fieldKind = "primitiveSlice"
	pointerSliceKind   fieldKind = "pointerSlice"
)

type dataType struct {
	Name          string
	RuntimeFields []string
	Fields        []dataField
}

type dataField struct {
	Name        string
	RuntimeName string
	DiffIndex   uint32
	Kind        fieldKind
	KeyType     string
	ValueType   string
	ElementType string
	RuntimeType string
	Tag         string

	ProtoType    string
	ProtoKeyType string
	ProtoName    string
	ProtoGoName  string
	ProtoGoType  string

	key   types.Type
	value types.Type
}

type sourceFile struct {
	PackageName  string
	GoPackage    string
	Imports      []sourceImport
	ProtoImports []string
	Types        []dataType
}

type sourceImport struct {
	Alias string
	Path  string
}

func Generate(dir, protoDir string) error {
	fileSet := token.NewFileSet()

	// 加载目录
	pkgs, err := packages.Load(&packages.Config{
		Dir:        dir,
		Fset:       fileSet,
		Mode:       packages.LoadAllSyntax | packages.NeedModule,
		BuildFlags: []string{"-tags=diff_fast"},
	}, ".")

	if err != nil {
		return err
	}

	pkg := pkgs[0]
	if len(pkg.Errors) != 0 {
		return pkg.Errors[0]
	}

	for _, file := range pkg.Syntax {
		sourcePath := fileSet.Position(file.Pos()).Filename
		protoImports := make(map[string]struct{})
		source := sourceFile{
			PackageName: pkg.Name,
			GoPackage:   pkg.Module.Path + "/pb/" + pkg.Name,
		}
		packageImports := map[string]string{
			diffPackagePath:  "diff",
			source.GoPackage: "pbData",
		}
		typeString := func(value types.Type) string {
			return types.TypeString(value, func(valuePkg *types.Package) string {
				if valuePkg.Path() == pkg.PkgPath {
					return ""
				}

				packageImports[valuePkg.Path()] = valuePkg.Name()
				return valuePkg.Name()
			})
		}

		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}

			for _, specification := range general.Specs {
				typeSpec := specification.(*ast.TypeSpec)
				typeName := pkg.TypesInfo.Defs[typeSpec.Name].(*types.TypeName)

				named, ok := typeName.Type().(*types.Named)
				if !ok {
					continue
				}

				structure, ok := named.Underlying().(*types.Struct)
				if !ok {
					continue
				}

				data := dataType{Name: named.Obj().Name()}
				for index := 0; index < structure.NumFields(); index++ {
					field := structure.Field(index)
					tag, exists := reflect.StructTag(structure.Tag(index)).Lookup("diff")
					if tag == "-" {
						declaration := typeString(field.Type())
						if !field.Embedded() {
							declaration = field.Name() + " " + declaration
						}

						if structure.Tag(index) != "" {
							declaration += " `" + structure.Tag(index) + "`"
						}

						data.RuntimeFields = append(data.RuntimeFields, declaration)
						continue
					}

					if !exists {
						continue
					}

					diffIndex, err := cast.ToUint32E(tag)
					if err != nil {
						return err
					}

					model := dataField{
						Name:        field.Name(),
						RuntimeName: field.Name(),
						DiffIndex:   diffIndex,
						Tag:         structure.Tag(index),
						ProtoName:   strcase.SnakeCase(field.Name()),
					}

					model.RuntimeName = string(model.RuntimeName[0]+'a'-'A') + model.RuntimeName[1:]
					model.ProtoGoName = strcase.UpperCamelCase(model.ProtoName)

					switch fieldType := types.Unalias(field.Type()).(type) {
					case *types.Basic:
						model.Kind = primitiveKind
						model.value = fieldType
						model.ValueType = typeString(fieldType)
						model.RuntimeType = "diff.Primitive[" + model.ValueType + "]"

						model.ProtoType = protoScalarType(fieldType)

					case *types.Pointer:
						model.Kind = pointerKind
						model.value = fieldType
						model.ValueType = typeString(fieldType)
						model.ElementType = typeString(fieldType.Elem())
						model.RuntimeType = "diff.Pointer[" + model.ValueType + "]"

					case *types.Map:
						model.key = fieldType.Key()
						model.value = fieldType.Elem()
						model.KeyType = typeString(fieldType.Key())
						model.ValueType = typeString(fieldType.Elem())

						model.ProtoKeyType = protoScalarType(fieldType.Key())
						switch model.ProtoKeyType {
						case "float", "double":
							panic("diffgen: Proto map 的 key 不支持浮点类型")
						}

						if pointer, ok := types.Unalias(fieldType.Elem()).(*types.Pointer); ok {
							model.Kind = pointerMapKind
							model.ElementType = typeString(pointer.Elem())
							model.RuntimeType = "diff.PointerMap[" + model.KeyType + ", " + model.ValueType + "]"
						} else {
							model.Kind = primitiveMapKind
							model.RuntimeType = "diff.PrimitiveMap[" + model.KeyType + ", " + model.ValueType + "]"
							model.ProtoType = protoScalarType(fieldType.Elem())
						}

					case *types.Slice:
						model.value = fieldType.Elem()
						model.ValueType = typeString(fieldType.Elem())

						if pointer, ok := types.Unalias(fieldType.Elem()).(*types.Pointer); ok {
							model.Kind = pointerSliceKind
							model.ElementType = typeString(pointer.Elem())
							model.RuntimeType = "diff.PointerSlice[" + model.ValueType + "]"
						} else {
							model.Kind = primitiveSliceKind
							model.RuntimeType = "diff.PrimitiveSlice[" + model.ValueType + "]"
							model.ProtoType = protoScalarType(fieldType.Elem())
						}

					default:
						panic("diffgen: 不支持的字段类型")
					}

					switch model.Kind {
					case pointerKind, pointerMapKind, pointerSliceKind:
						pointer := types.Unalias(model.value).(*types.Pointer)
						named := types.Unalias(pointer.Elem()).(*types.Named)
						if _, ok := named.Underlying().(*types.Struct); !ok {
							panic("diffgen: 对象指针必须指向结构体 " + named.String())
						}

						model.ProtoType = named.Obj().Name()
						targetPackage := named.Obj().Pkg()
						model.ProtoGoType = "*pbData." + named.Obj().Name()
						if targetPackage.Path() != pkg.PkgPath {
							model.ProtoType = targetPackage.Name() + "." + model.ProtoType
							if model.Kind != pointerKind {
								alias := "pb" + targetPackage.Name()
								packageImports[pkg.Module.Path+"/pb/"+targetPackage.Name()] = alias
								model.ProtoGoType = "*" + alias + "." + named.Obj().Name()
							}
						}
						targetPath := fileSet.Position(named.Obj().Pos()).Filename
						if targetPath != sourcePath {
							protoName := strings.TrimSuffix(filepath.Base(targetPath), ".go") + ".proto"
							importPath := filepath.ToSlash(filepath.Join(targetPackage.Name(), protoName))
							protoImports[importPath] = struct{}{}
						}
					default:
						model.ProtoGoType = model.ProtoType
						switch model.ProtoType {
						case "float":
							model.ProtoGoType = "float32"
						case "double":
							model.ProtoGoType = "float64"
						}
					}

					data.Fields = append(data.Fields, model)
				}

				if len(data.Fields) != 0 {
					source.Types = append(source.Types, data)
				}

			}
		}

		for path, alias := range packageImports {
			source.Imports = append(source.Imports, sourceImport{
				Alias: alias,
				Path:  path,
			})
		}
		sort.Slice(source.Imports, func(left, right int) bool {
			return source.Imports[left].Alias < source.Imports[right].Alias
		})
		if len(source.Types) == 0 {
			continue
		}

		for importPath := range protoImports {
			source.ProtoImports = append(source.ProtoImports, importPath)
		}
		sort.Strings(source.ProtoImports)

		var output bytes.Buffer
		if err := codeTemplate.ExecuteTemplate(&output, "file.go.tpl", source); err != nil {
			return err
		}
		code, err := format.Source(output.Bytes())
		if err != nil {
			return err
		}

		outputPath := strings.TrimSuffix(sourcePath, ".go") + "_diff.gen.go"

		if err := os.WriteFile(outputPath, code, 0644); err != nil {
			return err
		}

		output.Reset()
		if err := codeTemplate.ExecuteTemplate(&output, "file.proto.tpl", source); err != nil {
			return err
		}

		protoName := strings.TrimSuffix(filepath.Base(sourcePath), ".go") + ".proto"
		protoPath := filepath.Join(protoDir, pkg.Name, protoName)

		if err := os.MkdirAll(filepath.Dir(protoPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(protoPath, output.Bytes(), 0644); err != nil {
			return err
		}
	}

	return nil
}

func protoScalarType(value types.Type) string {
	switch value.Underlying().(*types.Basic).Kind() {
	case types.Bool:
		return "bool"
	case types.String:
		return "string"
	case types.Int8, types.Int16, types.Int32:
		return "int32"
	case types.Int, types.Int64:
		return "int64"
	case types.Uint8, types.Uint16, types.Uint32:
		return "uint32"
	case types.Uint, types.Uint64:
		return "uint64"
	case types.Float32:
		return "float"
	case types.Float64:
		return "double"
	default:
		panic("diffgen: 不支持的 Proto 基础类型 " + value.String())
	}
}

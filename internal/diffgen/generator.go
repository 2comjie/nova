package diffgen

import (
	"bytes"
	"embed"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"reflect"
	"sort"
	"text/template"

	"github.com/spf13/cast"
	"golang.org/x/tools/go/packages"
)

const diffPackagePath = "github.com/2comjie/nova/diff"

//go:embed templates/file.go.tpl
var templateFile embed.FS

var codeTemplate = template.Must(
	template.ParseFS(templateFile, "templates/file.go.tpl"),
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

	key   types.Type
	value types.Type
}

type sourceFile struct {
	PackageName string
	Imports     []sourceImport
	Types       []dataType
}

type sourceImport struct {
	Alias string
	Path  string
}

func Generate(dir string) error {
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
		source := sourceFile{
			PackageName: pkg.Name,
		}
		packageImports := map[string]string{
			diffPackagePath: "diff",
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
					}

					model.RuntimeName = string(model.RuntimeName[0]+'a'-'A') + model.RuntimeName[1:]

					switch fieldType := types.Unalias(field.Type()).(type) {
					case *types.Basic:
						model.Kind = primitiveKind
						model.value = fieldType
						model.ValueType = typeString(fieldType)
						model.RuntimeType = "diff.Primitive[" + model.ValueType + "]"

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

						if pointer, ok := types.Unalias(fieldType.Elem()).(*types.Pointer); ok {
							model.Kind = pointerMapKind
							model.ElementType = typeString(pointer.Elem())
							model.RuntimeType = "diff.PointerMap[" +
								model.KeyType + ", " + model.ValueType + "]"
						} else {
							model.Kind = primitiveMapKind
							model.RuntimeType = "diff.PrimitiveMap[" +
								model.KeyType + ", " + model.ValueType + "]"
						}

					case *types.Slice:
						model.value = fieldType.Elem()
						model.ValueType = typeString(fieldType.Elem())

						if pointer, ok := types.Unalias(fieldType.Elem()).(*types.Pointer); ok {
							model.Kind = pointerSliceKind
							model.ElementType = typeString(pointer.Elem())
							model.RuntimeType =
								"diff.PointerSlice[" + model.ValueType + "]"
						} else {
							model.Kind = primitiveSliceKind
							model.RuntimeType =
								"diff.PrimitiveSlice[" + model.ValueType + "]"
						}

					default:
						panic("diffgen: 不支持的字段类型")
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

		var output bytes.Buffer
		if err := codeTemplate.ExecuteTemplate(&output, "file.go.tpl", source); err != nil {
			return err
		}
		code, err := format.Source(output.Bytes())
		if err != nil {
			return err
		}
		fmt.Println(string(code))
	}

	return nil
}

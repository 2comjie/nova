package diffgen

import (
	"bytes"
	"embed"
	"errors"
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
	"golang.org/x/tools/imports"
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
	TypeParams    string
	TypeArgs      string
	Instance      string
	CodecName     string
	JSONPackage   string
	RuntimeFields []string
	Fields        []dataField
}

type dataField struct {
	Name           string
	RuntimeName    string
	DiffIndex      uint32
	Kind           fieldKind
	KeyType        string
	ValueType      string
	ElementType    string
	RuntimeType    string
	Tag            string
	CodecKeyType   string
	CodecValueType string
	IsTime         bool
	TimePackage    string

	ProtoType     string
	ProtoKeyType  string
	ProtoName     string
	ProtoGoName   string
	ProtoGoType   string
	ChildCodec    string
	ChildName     string
	UpdateIndexes [4]uint32

	key   types.Type
	value types.Type
}

type sourceFile struct {
	PackageName     string
	ProtoPackage    string
	GoPackage       string
	CSharpNamespace string
	Imports         []sourceImport
	ProtoImports    []string
	Types           []dataType
	ProtoTypes      []dataType
	Declarations    []string
}

// ChildCall 在普通对象方法和具体实例的协议转换函数之间选择调用方式。
func (field dataField) ChildCall(method, receiver string, args ...string) string {
	if field.ChildName != "" {
		return field.ChildCodec + method + field.ChildName + "(" + strings.Join(append([]string{receiver}, args...), ", ") + ")"
	}
	return receiver + "." + method + "(" + strings.Join(args, ", ") + ")"
}

func isTime(value types.Type) bool {
	named, ok := types.Unalias(value).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "time" && named.Obj().Name() == "Time"
}

func (field dataField) Encode(expression string) string {
	if field.IsTime {
		return expression + ".UnixMilli()"
	}
	if field.ValueType != field.ProtoGoType {
		return field.ProtoGoType + "(" + expression + ")"
	}
	return expression
}

func (field dataField) Decode(expression string) string {
	if field.IsTime {
		return field.TimePackage + ".UnixMilli(" + expression + ").UTC()"
	}
	if field.ValueType != field.ProtoGoType {
		return field.ValueType + "(" + expression + ")"
	}
	return expression
}

type sourceImport struct {
	Alias string
	Path  string
}

func (config config) generate() error {
	fileSet := token.NewFileSet()

	// 加载目录
	pkgs, err := packages.Load(&packages.Config{
		Dir:        config.dir,
		Fset:       fileSet,
		Mode:       packages.LoadAllSyntax | packages.NeedModule,
		BuildFlags: []string{"-tags=diff_fast"},
	}, config.Packages...)

	if err != nil {
		return err
	}

	targets := pkgs
	for _, pkg := range targets {
		if len(pkg.Errors) != 0 {
			return pkg.Errors[0]
		}
	}
	if len(targets) == 0 {
		return errors.New("diffgen: packages 没有匹配任何 Go 包")
	}
	module := targets[0].Module
	if module == nil {
		return errors.New("diffgen: 源码必须属于 Go 模块")
	}
	outputRel, err := filepath.Rel(module.Dir, config.Proto.GoOut)
	if err != nil {
		return err
	}
	if outputRel == ".." || strings.HasPrefix(outputRel, ".."+string(filepath.Separator)) {
		return errors.New("diffgen: proto.go_out 必须在源码所属 Go 模块内")
	}
	goPrefix := module.Path
	if outputRel != "." {
		goPrefix += "/" + filepath.ToSlash(outputRel)
	}
	instances := make(map[*types.Named]*types.Named)
	protoGoPackages := make(map[string]string)
	packagePaths := make(map[string]string)
	protoPackages := make(map[string]string)
	packages.Visit(pkgs, nil, func(loaded *packages.Package) {
		if len(loaded.GoFiles) != 0 {
			rel, err := filepath.Rel(config.SourceRoot, filepath.Dir(loaded.GoFiles[0]))
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				packagePaths[loaded.PkgPath] = rel
				protoGoPackages[loaded.PkgPath] = goPrefix + "/" + filepath.ToSlash(filepath.Join("pb", rel))
				name := filepath.ToSlash(rel)
				if rel == "." {
					name = loaded.Name
				}
				protoPackages[loaded.PkgPath] = strings.ReplaceAll(strings.ReplaceAll(name, "/", "."), "-", "_")
			}
		}
		for _, file := range loaded.Syntax {
			for _, decl := range file.Decls {
				general, ok := decl.(*ast.GenDecl)
				if !ok || general.Tok != token.TYPE {
					continue
				}
				for _, spec := range general.Specs {
					definition := spec.(*ast.TypeSpec)
					named, ok := loaded.TypesInfo.Defs[definition.Name].Type().(*types.Named)
					if !ok || named.TypeParams().Len() != 0 {
						continue
					}
					origin, ok := types.Unalias(loaded.TypesInfo.TypeOf(definition.Type)).(*types.Named)
					if ok && origin.TypeArgs().Len() != 0 {
						instances[named] = origin
					}
				}
			}
		}
	})

	for _, pkg := range targets {
		if _, exists := packagePaths[pkg.PkgPath]; !exists || pkg.Module == nil || pkg.Module.Path != module.Path {
			return errors.New("diffgen: 生成包必须位于 source_root 和同一 Go 模块内：" + pkg.PkgPath)
		}
		for _, file := range pkg.Syntax {
			sourcePath := fileSet.Position(file.Pos()).Filename
			protoImports := make(map[string]struct{})
			namespace := strings.Split(protoPackages[pkg.PkgPath], ".")
			for index := range namespace {
				namespace[index] = strcase.UpperCamelCase(namespace[index])
			}
			source := sourceFile{
				PackageName:     pkg.Name,
				ProtoPackage:    protoPackages[pkg.PkgPath],
				GoPackage:       protoGoPackages[pkg.PkgPath],
				CSharpNamespace: "Nova.Generated." + strings.Join(namespace, "."),
			}
			packageImports := map[string]string{
				diffPackagePath:  "diff",
				source.GoPackage: "pbData",
				"encoding/json":  "json",
			}
			addImport := func(path, name string) string {
				if alias, exists := packageImports[path]; exists {
					return alias
				}
				alias := name
				for index := 2; ; index++ {
					used := false
					for _, current := range packageImports {
						if current == alias {
							used = true
							break
						}
					}
					if !used {
						packageImports[path] = alias
						return alias
					}
					alias = name + cast.ToString(index)
				}
			}
			typeString := func(value types.Type) string {
				return types.TypeString(value, func(valuePkg *types.Package) string {
					if valuePkg.Path() == pkg.PkgPath {
						return ""
					}

					return addImport(valuePkg.Path(), valuePkg.Name())
				})
			}

			exclusive := false
			for _, group := range file.Comments {
				for _, comment := range group.List {
					if strings.HasPrefix(comment.Text, "//go:build ") && strings.Contains(comment.Text, "diff_fast") {
						exclusive = true
					}
				}
			}
			if exclusive {
				for _, declaration := range file.Decls {
					general, ok := declaration.(*ast.GenDecl)
					if !ok {
						continue
					}
					var declarations []ast.Node
					if general.Tok == token.CONST {
						declarations = append(declarations, general)
					}
					if general.Tok == token.TYPE {
						for _, specification := range general.Specs {
							typeSpec := specification.(*ast.TypeSpec)
							_, structure := pkg.TypesInfo.Defs[typeSpec.Name].Type().Underlying().(*types.Struct)
							if typeSpec.Assign.IsValid() || !structure {
								declarations = append(declarations, &ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{typeSpec}})
							}
						}
					}
					for _, node := range declarations {
						var output bytes.Buffer
						if err := format.Node(&output, fileSet, node); err != nil {
							return err
						}
						source.Declarations = append(source.Declarations, output.String())
						ast.Inspect(node, func(node ast.Node) bool {
							if ident, ok := node.(*ast.Ident); ok {
								if imported, ok := pkg.TypesInfo.Uses[ident].(*types.PkgName); ok {
									packageImports[imported.Imported().Path()] = imported.Name()
								}
							}
							return true
						})
					}
				}
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

					data := dataType{Name: named.Obj().Name(), CodecName: "_diff" + named.Obj().Name() + "Data", JSONPackage: packageImports["encoding/json"]}
					if origin := instances[named]; origin != nil {
						data.Instance = typeString(origin)
					}
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

						diffIndex, err := cast.ToUint64E(tag)
						if err != nil {
							return err
						}
						if diffIndex == 0 || diffIndex >= 1000 {
							panic("diffgen: diff index 必须在 1..999，增量字段使用后续千位段")
						}

						model := dataField{
							Name:        field.Name(),
							RuntimeName: field.Name(),
							DiffIndex:   uint32(diffIndex),
							Tag:         structure.Tag(index),
							ProtoName:   strcase.SnakeCase(field.Name()),
						}
						for segment := range model.UpdateIndexes {
							model.UpdateIndexes[segment] = uint32(diffIndex) + uint32(segment+1)*1000
						}

						model.RuntimeName = string(model.RuntimeName[0]+'a'-'A') + model.RuntimeName[1:]
						if token.Lookup(model.RuntimeName).IsKeyword() {
							model.RuntimeName += "_"
						}
						model.ProtoGoName = strcase.UpperCamelCase(model.ProtoName)

						underlying := field.Type().Underlying()
						if isTime(field.Type()) {
							underlying = types.Typ[types.Int64]
						}
						if origin := instances[named]; origin != nil {
							template := origin.Origin().Underlying().(*types.Struct)
							if _, parameter := types.Unalias(template.Field(index).Type()).(*types.TypeParam); parameter {
								switch underlying.(type) {
								case *types.Basic, *types.Pointer:
								default:
									panic("diffgen: 泛型值参数只支持基础值或对象指针：" + field.Type().String())
								}
							}
						}
						switch fieldType := underlying.(type) {
						case *types.Interface:
							param, ok := types.Unalias(field.Type()).(*types.TypeParam)
							if !ok {
								panic("diffgen: diff 字段不能是接口 " + field.Name())
							}
							// 泛型模板只需要字段访问代码，具体实例再决定 Proto 类型。
							model.Kind = primitiveKind
							model.value = param
							model.ValueType = typeString(param)
							model.RuntimeType = "diff.Value[" + model.ValueType + "]"
						case *types.Basic:
							model.Kind = primitiveKind
							model.value = field.Type()
							model.ValueType = typeString(field.Type())
							model.RuntimeType = "diff.Value[" + model.ValueType + "]"
							model.ProtoType = protoScalarType(fieldType)
						case *types.Pointer:
							model.Kind = pointerKind
							model.value = fieldType
							model.ValueType = typeString(fieldType)
							model.ElementType = typeString(fieldType.Elem())
							model.RuntimeType = "diff.Value[" + model.ValueType + "]"
						case *types.Map:
							model.key = fieldType.Key()
							model.value = fieldType.Elem()
							model.KeyType = typeString(fieldType.Key())
							model.CodecKeyType = model.KeyType
							if basic, ok := fieldType.Key().Underlying().(*types.Basic); ok && basic.Kind() == types.Bool {
								model.CodecKeyType = "diff.BoolKey"
							}
							model.ValueType = typeString(fieldType.Elem())
							model.RuntimeType = "diff.Map[" + model.KeyType + ", " + model.ValueType + "]"
							if _, param := types.Unalias(fieldType.Key()).(*types.TypeParam); !param {
								if isTime(fieldType.Key()) {
									panic("diffgen: Proto map 的 key 不支持 time.Time")
								}
								model.ProtoKeyType = protoScalarType(fieldType.Key())
								switch model.ProtoKeyType {
								case "float", "double":
									panic("diffgen: Proto map 的 key 不支持浮点类型")
								}
							}
							if pointer, ok := types.Unalias(fieldType.Elem()).(*types.Pointer); ok {
								model.Kind = pointerMapKind
								model.ElementType = typeString(pointer.Elem())
							} else {
								model.Kind = primitiveMapKind
								if _, param := types.Unalias(fieldType.Elem()).(*types.TypeParam); !param {
									model.ProtoType = protoScalarType(fieldType.Elem())
								}
							}
						case *types.Slice:
							model.value = fieldType.Elem()
							model.ValueType = typeString(fieldType.Elem())
							model.RuntimeType = "diff.Slice[" + model.ValueType + "]"
							if pointer, ok := types.Unalias(fieldType.Elem()).(*types.Pointer); ok {
								model.Kind = pointerSliceKind
								model.ElementType = typeString(pointer.Elem())
							} else {
								model.Kind = primitiveSliceKind
								if _, param := types.Unalias(fieldType.Elem()).(*types.TypeParam); !param {
									model.ProtoType = protoScalarType(fieldType.Elem())
								}
							}
						default:
							panic("diffgen: 不支持的字段类型")
						}
						model.IsTime = isTime(model.value)
						model.CodecValueType = model.ValueType
						if model.IsTime {
							model.CodecValueType = "int64"
							if _, exists := packageImports["time"]; !exists {
								packageImports["time"] = "time"
							}
							model.TimePackage = packageImports["time"]
						}

						switch model.Kind {
						case pointerKind, pointerMapKind, pointerSliceKind:
							if named.TypeParams().Len() != 0 {
								break
							}
							pointer := types.Unalias(model.value).(*types.Pointer)
							named := types.Unalias(pointer.Elem()).(*types.Named)
							if named.TypeArgs().Len() != 0 {
								panic("diffgen: 泛型对象请先声明具体类型：type Name " + named.String())
							}
							if _, ok := named.Underlying().(*types.Struct); !ok {
								panic("diffgen: 对象指针必须指向结构体 " + named.String())
							}

							model.ProtoType = named.Obj().Name()
							if instances[named] != nil {
								model.ChildName = named.Obj().Name()
								if named.Obj().Pkg().Path() != pkg.PkgPath {
									model.ChildCodec = strings.TrimSuffix(typeString(named), named.Obj().Name())
								}
							}
							targetPackage := named.Obj().Pkg()
							if _, exists := packagePaths[targetPackage.Path()]; !exists {
								return errors.New("diffgen: Proto 对象必须位于 source_root 内：" + named.String())
							}
							model.ProtoGoType = "*pbData." + named.Obj().Name()
							if targetPackage.Path() != pkg.PkgPath {
								model.ProtoType = "." + protoPackages[targetPackage.Path()] + "." + model.ProtoType
								alias := addImport(protoGoPackages[targetPackage.Path()], "pb"+targetPackage.Name())
								model.ProtoGoType = "*" + alias + "." + named.Obj().Name()
							}
							targetPath := fileSet.Position(named.Obj().Pos()).Filename
							if targetPath != sourcePath {
								protoName := strings.TrimSuffix(filepath.Base(targetPath), ".go") + ".proto"
								importPath := filepath.ToSlash(filepath.Join(packagePaths[targetPackage.Path()], protoName))
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

						if data.Instance != "" {
							model.RuntimeName = "DiffField(" + cast.ToString(model.DiffIndex) + ").(*" + model.RuntimeType + ")"
						}
						data.Fields = append(data.Fields, model)
					}
					if named.TypeParams().Len() != 0 {
						var params, args []string
						for index := 0; index < named.TypeParams().Len(); index++ {
							param := named.TypeParams().At(index)
							constraint := typeString(param.Constraint())
							params = append(params, param.Obj().Name()+" "+constraint)
							args = append(args, param.Obj().Name())
						}
						data.TypeParams = "[" + strings.Join(params, ", ") + "]"
						data.TypeArgs = "[" + strings.Join(args, ", ") + "]"
					}

					if len(data.Fields) != 0 {
						source.Types = append(source.Types, data)
						if data.TypeParams == "" {
							source.ProtoTypes = append(source.ProtoTypes, data)
						}
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
			if len(source.Types) == 0 && len(source.Declarations) == 0 {
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
			outputPath := strings.TrimSuffix(sourcePath, ".go") + "_diff.gen.go"
			code, err := imports.Process(outputPath, output.Bytes(), nil)
			if err != nil {
				return err
			}

			if err := os.WriteFile(outputPath, code, 0644); err != nil {
				return err
			}
			if len(source.ProtoTypes) == 0 {
				continue
			}

			output.Reset()
			if err := codeTemplate.ExecuteTemplate(&output, "file.proto.tpl", source); err != nil {
				return err
			}

			protoName := strings.TrimSuffix(filepath.Base(sourcePath), ".go") + ".proto"
			protoPath := filepath.Join(config.Proto.Dir, packagePaths[pkg.PkgPath], protoName)

			if err := os.MkdirAll(filepath.Dir(protoPath), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(protoPath, output.Bytes(), 0644); err != nil {
				return err
			}
		}
	}

	return nil
}

func protoScalarType(value types.Type) string {
	if isTime(value) {
		return "int64"
	}
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

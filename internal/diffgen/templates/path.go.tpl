{{define "path"}}{{range .Types}}{{$type := .}}
type {{.Name}}DiffPath{{.PathTypeParams}} struct {
	path {{.DiffAlias}}.PathBuilder[{{.PathRoot}}]
}

func New{{.Name}}DiffPath{{.PathTypeParams}}(path {{.DiffAlias}}.PathBuilder[{{.PathRoot}}]) {{.Name}}DiffPath{{.PathTypeArgs}} {
	return {{.Name}}DiffPath{{.PathTypeArgs}}{path: path}
}
{{if .Generic}}
func {{.Name}}Diff{{.TypeParams}}() {{.Name}}DiffPath{{.PathRootTypeArgs}} {
	return New{{.Name}}DiffPath{{.PathRootTypeArgs}}({{.DiffAlias}}.NewPathBuilder[*{{.Receiver}}]())
}
{{else}}
var {{.Name}}Diff = New{{.Name}}DiffPath{{.PathRootTypeArgs}}({{.DiffAlias}}.NewPathBuilder[*{{.Receiver}}]())
{{end}}
{{range .Fields}}{{if .Primitive}}
func (path {{$type.Name}}DiffPath{{$type.PathTypeArgs}}) {{.Name}}() {{.DiffAlias}}.ValuePath[{{$type.PathRoot}}, {{.ValueType}}] {
	return {{.DiffAlias}}.NewValuePath[{{$type.PathRoot}}, {{.ValueType}}](path.path.Field({{.DiffIndex}}))
}
{{else if .Pointer}}
func (path {{$type.Name}}DiffPath{{$type.PathTypeArgs}}) {{.Name}}() {{.PathContainer}}{{$type.PathTypeArgs}} {
	fieldPath := path.path.Field({{.DiffIndex}})
	return {{.PathContainer}}{{$type.PathTypeArgs}}{
		{{.PathConstructor}}(fieldPath),
		{{.DiffAlias}}.NewValuePath[{{$type.PathRoot}}, {{.ValueType}}](fieldPath),
	}
}

type {{.PathContainer}}{{$type.PathTypeParams}} struct {
	{{.PathType}}
	value {{.DiffAlias}}.ValuePath[{{$type.PathRoot}}, {{.ValueType}}]
}

func (path {{.PathContainer}}{{$type.PathTypeArgs}}) Changes() {{.DiffAlias}}.ValuePath[{{$type.PathRoot}}, {{.ValueType}}] {
	return path.value
}
{{else if .PrimitiveMap}}
func (path {{$type.Name}}DiffPath{{$type.PathTypeArgs}}) {{.Name}}() {{.DiffAlias}}.MapPath[{{$type.PathRoot}}, {{.KeyType}}, {{.ValueType}}] {
	return {{.DiffAlias}}.NewMapPath[{{$type.PathRoot}}, {{.KeyType}}, {{.ValueType}}](path.path, {{.DiffIndex}})
}
{{else if .PointerMap}}
func (path {{$type.Name}}DiffPath{{$type.PathTypeArgs}}) {{.Name}}() {{.PathContainer}}{{$type.PathTypeArgs}} {
	return {{.PathContainer}}{{$type.PathTypeArgs}}{
		path: {{.DiffAlias}}.NewMapPath[{{$type.PathRoot}}, {{.KeyType}}, {{.ValueType}}](path.path, {{.DiffIndex}}),
	}
}

type {{.PathContainer}}{{$type.PathTypeParams}} struct {
	path {{.DiffAlias}}.MapPath[{{$type.PathRoot}}, {{.KeyType}}, {{.ValueType}}]
}

func (path {{.PathContainer}}{{$type.PathTypeArgs}}) Changes() {{.DiffAlias}}.MapPath[{{$type.PathRoot}}, {{.KeyType}}, {{.ValueType}}] {
	return path.path
}

func (path {{.PathContainer}}{{$type.PathTypeArgs}}) Any() {{.PathType}} {
	return {{.PathConstructor}}(path.path.AnyPath())
}

func (path {{.PathContainer}}{{$type.PathTypeArgs}}) Key(key {{.KeyType}}) {{.PathType}} {
	return {{.PathConstructor}}(path.path.KeyPath(key))
}
{{else if .PrimitiveSlice}}
func (path {{$type.Name}}DiffPath{{$type.PathTypeArgs}}) {{.Name}}() {{.DiffAlias}}.SlicePath[{{$type.PathRoot}}, {{.ValueType}}] {
	return {{.DiffAlias}}.NewSlicePath[{{$type.PathRoot}}, {{.ValueType}}](path.path, {{.DiffIndex}})
}
{{else if .PointerSlice}}
func (path {{$type.Name}}DiffPath{{$type.PathTypeArgs}}) {{.Name}}() {{.PathContainer}}{{$type.PathTypeArgs}} {
	return {{.PathContainer}}{{$type.PathTypeArgs}}{
		path: {{.DiffAlias}}.NewSlicePath[{{$type.PathRoot}}, {{.ValueType}}](path.path, {{.DiffIndex}}),
	}
}

type {{.PathContainer}}{{$type.PathTypeParams}} struct {
	path {{.DiffAlias}}.SlicePath[{{$type.PathRoot}}, {{.ValueType}}]
}

func (path {{.PathContainer}}{{$type.PathTypeArgs}}) Changes() {{.DiffAlias}}.SlicePath[{{$type.PathRoot}}, {{.ValueType}}] {
	return path.path
}

func (path {{.PathContainer}}{{$type.PathTypeArgs}}) Any() {{.PathType}} {
	return {{.PathConstructor}}(path.path.AnyPath())
}

func (path {{.PathContainer}}{{$type.PathTypeArgs}}) Index(index int) {{.PathType}} {
	return {{.PathConstructor}}(path.path.IndexPath(index))
}
{{end}}{{end}}{{end}}{{end}}

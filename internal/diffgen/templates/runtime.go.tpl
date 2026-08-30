{{define "runtime"}}{{if .Schema}}{{range .Types}}{{$type := .}}
type {{.Name}}{{.TypeParams}} struct {
	{{.DiffAlias}}.Object
{{range .Fields}}	{{.RuntimeName}} {{.RuntimeType}}
{{end}}}
{{range .Fields}}{{if .Primitive}}
func (value *{{$type.Receiver}}) Get{{.Name}}() {{.ValueType}} {
	return value.{{.RuntimeName}}.GetValue()
}

func (value *{{$type.Receiver}}) Set{{.Name}}(fieldValue {{.ValueType}}) bool {
	value.EnsureDiffLink()
	return value.{{.RuntimeName}}.SetValue(fieldValue)
}
{{else if .Pointer}}
func (value *{{$type.Receiver}}) Get{{.Name}}() {{.ValueType}} {
	return value.{{.RuntimeName}}.GetValue()
}

func (value *{{$type.Receiver}}) Set{{.Name}}(fieldValue {{.ValueType}}) bool {
	value.EnsureDiffLink()
	return value.{{.RuntimeName}}.SetValue(fieldValue)
}

func (value *{{$type.Receiver}}) Clear{{.Name}}() bool {
	value.EnsureDiffLink()
	return value.{{.RuntimeName}}.SetValue(nil)
}
{{else if .Collection}}
func (value *{{$type.Receiver}}) {{.Name}}() *{{.RuntimeType}} {
	value.EnsureDiffLink()
	return &value.{{.RuntimeName}}
}
{{end}}{{end}}{{end}}{{end}}{{end}}

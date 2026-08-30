{{define "link"}}{{range .Types}}
func (value *{{.Receiver}}) InitLink(writer *{{.DiffAlias}}.Writer) {
	if writer != nil {
		{{.DiffAlias}}.BindWriter[*{{.Receiver}}](writer)
	}
	value.InitDiffLink(writer, make(map[*{{.DiffAlias}}.Object]struct{}))
}

func (value *{{.Receiver}}) InitDiffLink(writer *{{.DiffAlias}}.Writer, visited map[*{{.DiffAlias}}.Object]struct{}) {
	if value == nil {
		return
	}
	if visited == nil {
		visited = make(map[*{{.DiffAlias}}.Object]struct{})
	}
	if _, exists := visited[&value.Object]; exists {
		return
	}
	visited[&value.Object] = struct{}{}
	value.Object.Init(writer)
{{range .Fields}}{{if .Pointer}}	if child := value.{{.RuntimeName}}.GetValue(); child != nil {
		child.InitDiffLink(nil, visited)
	}
{{else if .PointerMap}}	value.{{.RuntimeName}}.Range(func(_ {{.KeyType}}, child {{.ValueType}}) bool {
		if child != nil {
			child.InitDiffLink(nil, visited)
		}
		return true
	})
{{else if .PointerSlice}}	value.{{.RuntimeName}}.Range(func(_ int, child {{.ValueType}}) bool {
		if child != nil {
			child.InitDiffLink(nil, visited)
		}
		return true
	})
{{end}}	value.{{.RuntimeName}}.Init(&value.Object, {{.DiffIndex}})
{{end}}}

func (value *{{.Receiver}}) AppendDiffValue(data []byte) []byte {
{{range .Fields}}	data = value.{{.RuntimeName}}.AppendValue(data, {{.DiffIndex}})
{{end}}	return data
}
{{end}}{{end}}

{{define "codecs"}}
// {{.CodecName}}{{.TypeArgs}} 仅包含业务数据；子对象使用自己的编解码方法。
type {{.CodecName}}{{.TypeParams}} struct {
{{range .Fields}}
    {{.Name}} {{if or (eq .Kind "primitiveMap") (eq .Kind "pointerMap")}}map[{{.CodecKeyType}}]{{.CodecValueType}}{{else if or (eq .Kind "primitiveSlice") (eq .Kind "pointerSlice")}}[]{{.CodecValueType}}{{else}}{{.CodecValueType}}{{end}}{{if .Tag}} `{{.Tag}}`{{end}}
{{end}}
}

func (value *{{.Name}}{{.TypeArgs}}) exportData() {{.CodecName}}{{.TypeArgs}} {
    data := {{.CodecName}}{{.TypeArgs}}{
{{range .Fields}}
{{- if or (eq .Kind "primitive") (eq .Kind "pointer")}}
        {{.Name}}: {{if .IsTime}}{{.Encode (printf "value.%s.GetValue()" .RuntimeName)}}{{else}}value.{{.RuntimeName}}.GetValue(){{end}},
{{- end}}
{{end}}
    }
{{range .Fields}}
{{- if or (eq .Kind "primitiveMap") (eq .Kind "pointerMap")}}
    if value.{{.RuntimeName}}.Len() != 0 {
        data.{{.Name}} = make(map[{{.CodecKeyType}}]{{.CodecValueType}}, value.{{.RuntimeName}}.Len())
        value.{{.RuntimeName}}.Range(func(key {{.KeyType}}, fieldValue {{.ValueType}}) bool {
            data.{{.Name}}[{{if ne .KeyType .CodecKeyType}}{{.CodecKeyType}}({{end}}key{{if ne .KeyType .CodecKeyType}}){{end}}] = {{if .IsTime}}{{.Encode "fieldValue"}}{{else}}fieldValue{{end}}
            return true
        })
    }
{{- else if or (eq .Kind "primitiveSlice") (eq .Kind "pointerSlice")}}
    if value.{{.RuntimeName}}.Len() != 0 {
        data.{{.Name}} = make([]{{.CodecValueType}}, value.{{.RuntimeName}}.Len())
        for index := range data.{{.Name}} {
            data.{{.Name}}[index] = {{if .IsTime}}{{.Encode (printf "value.%s.GetValue(index)" .RuntimeName)}}{{else}}value.{{.RuntimeName}}.GetValue(index){{end}}
        }
    }
{{- end}}
{{end}}
    return data
}

// loadData 接管完整解码成功的数据，不记录增量。
func (value *{{.Name}}{{.TypeArgs}}) loadData(data {{.CodecName}}{{.TypeArgs}}) {
    value.InitLink(nil)
{{range .Fields}}
{{- if and (or (eq .Kind "primitiveMap") (eq .Kind "pointerMap")) (or (ne .KeyType .CodecKeyType) .IsTime)}}
    {
        var values map[{{.KeyType}}]{{.ValueType}}
        if len(data.{{.Name}}) != 0 {
            values = make(map[{{.KeyType}}]{{.ValueType}}, len(data.{{.Name}}))
            for key, fieldValue := range data.{{.Name}} {
                values[{{.KeyType}}(key)] = {{if .IsTime}}{{.Decode "fieldValue"}}{{else}}fieldValue{{end}}
            }
        }
        value.{{.RuntimeName}}.LoadSnapshot(values)
    }
{{- else if and (eq .Kind "primitiveSlice") .IsTime}}
    {
        var values []{{.ValueType}}
        if len(data.{{.Name}}) != 0 {
            values = make([]{{.ValueType}}, len(data.{{.Name}}))
            for index, fieldValue := range data.{{.Name}} {
                values[index] = {{.Decode "fieldValue"}}
            }
        }
        value.{{.RuntimeName}}.LoadSnapshot(values)
    }
{{- else}}
    value.{{.RuntimeName}}.LoadSnapshot({{if .IsTime}}{{.Decode (printf "data.%s" .Name)}}{{else}}data.{{.Name}}{{end}})
{{- end}}
{{end}}
}

func (value *{{.Name}}{{.TypeArgs}}) MarshalJSON() ([]byte, error) {
    return {{.JSONPackage}}.Marshal(value.exportData())
}

func (value *{{.Name}}{{.TypeArgs}}) UnmarshalJSON(data []byte) error {
    var decoded {{.CodecName}}{{.TypeArgs}}
    if err := {{.JSONPackage}}.Unmarshal(data, &decoded); err != nil {
        return err
    }
    value.loadData(decoded)
    return nil
}

{{end}}

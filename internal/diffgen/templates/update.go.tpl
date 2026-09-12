{{define "updates"}}
{{$typeName := .Name}}
// Commit 导出本轮增量并清空 Writer；返回值独立于运行时对象。
{{if .Instance}}
func Commit{{.Name}}(value *{{.Name}}) *pbData.{{.Name}} {
    return value.Object.Commit(new(pbData.{{.Name}}), func(update *pbData.{{.Name}}, path diff.Path, operation diff.Operation, data any) {
        WriteUpdate{{.Name}}(value, update, path, operation, data)
    })
}
{{else}}
func (value *{{.Name}}) Commit() *pbData.{{.Name}} {
    return value.Object.Commit(new(pbData.{{.Name}}), value.WriteUpdate)
}
{{end}}

// WriteUpdate 将 Writer 的内部变更转换为有类型的 Proto 字段。
{{if .Instance}}func WriteUpdate{{.Name}}(value *{{.Name}}, update{{else}}func (value *{{.Name}}) WriteUpdate(update{{end}} *pbData.{{.Name}}, path diff.Path, operation diff.Operation, data any) {
    node := path[0]
    switch node.FieldIndex {
{{range .Fields}}
    case {{.DiffIndex}}:
{{- if eq .Kind "primitive"}}
        fieldValue := {{.Encode (printf "data.(%s)" .ValueType)}}
        update.{{.ProtoGoName}}Update = &fieldValue
{{- else if eq .Kind "pointer"}}
        if len(path) == 1 {
            if operation == diff.FieldSet {
                update.{{.ProtoGoName}}Diff = &pbData.{{$typeName}}_{{.ProtoGoName}}Set{ {{.ProtoGoName}}Set: {{.ChildCall "Snapshot" (printf "data.(%s)" .ValueType)}} }
            } else {
                update.{{.ProtoGoName}}Diff = &pbData.{{$typeName}}_{{.ProtoGoName}}Clear{ {{.ProtoGoName}}Clear: true }
            }
            return
        }
        childUpdate := update.Get{{.ProtoGoName}}Update()
        if childUpdate == nil {
            childUpdate = new({{slice .ProtoGoType 1}})
            update.{{.ProtoGoName}}Diff = &pbData.{{$typeName}}_{{.ProtoGoName}}Update{ {{.ProtoGoName}}Update: childUpdate }
        }
        {{.ChildCall "WriteUpdate" (printf "value.%s.GetValue()" .RuntimeName) "childUpdate" "path[1:]" "operation" "data"}}
{{- else if or (eq .Kind "primitiveMap") (eq .Kind "pointerMap")}}
        if node.KeyType == diff.PathField {
            update.{{.ProtoGoName}}Clear = true
            return
        }
        key := node.MapKey.({{.KeyType}})
{{- if eq .Kind "pointerMap"}}
        if len(path) > 1 {
            child, _ := value.{{.RuntimeName}}.Load(key)
            childUpdate := update.{{.ProtoGoName}}Update[{{if ne .KeyType .ProtoKeyType}}{{.ProtoKeyType}}({{end}}key{{if ne .KeyType .ProtoKeyType}}){{end}}]
            if childUpdate == nil {
                childUpdate = new({{slice .ProtoGoType 1}})
                diff.SetMap(&update.{{.ProtoGoName}}Update, {{if ne .KeyType .ProtoKeyType}}{{.ProtoKeyType}}({{end}}key{{if ne .KeyType .ProtoKeyType}}){{end}}, childUpdate)
            }
            {{.ChildCall "WriteUpdate" "child" "childUpdate" "path[1:]" "operation" "data"}}
            return
        }
{{- end}}
        if operation == diff.MapSet {
{{- if eq .Kind "pointerMap"}}
            diff.SetMap(&update.{{.ProtoGoName}}Set, {{if ne .KeyType .ProtoKeyType}}{{.ProtoKeyType}}({{end}}key{{if ne .KeyType .ProtoKeyType}}){{end}}, {{.ChildCall "Snapshot" (printf "data.(%s)" .ValueType)}})
{{- else}}
            diff.SetMap(&update.{{.ProtoGoName}}Set, {{if ne .KeyType .ProtoKeyType}}{{.ProtoKeyType}}({{end}}key{{if ne .KeyType .ProtoKeyType}}){{end}}, {{.Encode (printf "data.(%s)" .ValueType)}})
{{- end}}
        } else {
            update.{{.ProtoGoName}}Delete = append(update.{{.ProtoGoName}}Delete, {{if ne .KeyType .ProtoKeyType}}{{.ProtoKeyType}}({{end}}key{{if ne .KeyType .ProtoKeyType}}){{end}})
        }
{{- else}}
        update.{{.ProtoGoName}}Updated = true
        values := data.([]{{.ValueType}})
        update.{{.ProtoGoName}}Update = make([]{{.ProtoGoType}}, len(values))
        for index, fieldValue := range values {
{{- if eq .Kind "pointerSlice"}}
            if fieldValue != nil {
                update.{{.ProtoGoName}}Update[index] = {{.ChildCall "Snapshot" "fieldValue"}}
            }
{{- else}}
            update.{{.ProtoGoName}}Update[index] = {{.Encode "fieldValue"}}
{{- end}}
        }
{{- end}}
{{end}}
    }
}

// Merge 按序应用增量。对象必须已加载对应基线；绑定 Writer 时会记录本地变化。
{{if .Instance}}func Merge{{.Name}}(value *{{.Name}}, update{{else}}func (value *{{.Name}}) Merge(update{{end}} *pbData.{{.Name}}) {
    value.InitLink(nil)
{{range .Fields}}
{{- if eq .Kind "primitive"}}
    if update.{{.ProtoGoName}}Update != nil {
        value.{{.RuntimeName}}.SetValue({{.Decode (printf "*update.%sUpdate" .ProtoGoName)}})
    }
{{- else if eq .Kind "pointer"}}
    switch fieldValue := update.{{.ProtoGoName}}Diff.(type) {
    case *pbData.{{$typeName}}_{{.ProtoGoName}}Set:
        child := new({{.ElementType}})
        {{.ChildCall "LoadSnapshot" "child" (printf "fieldValue.%sSet" .ProtoGoName)}}
        value.{{.RuntimeName}}.SetValue(child)
    case *pbData.{{$typeName}}_{{.ProtoGoName}}Clear:
        value.{{.RuntimeName}}.SetValue(nil)
    case *pbData.{{$typeName}}_{{.ProtoGoName}}Update:
        {{.ChildCall "Merge" (printf "value.%s.GetValue()" .RuntimeName) (printf "fieldValue.%sUpdate" .ProtoGoName)}}
    }
{{- else if or (eq .Kind "primitiveMap") (eq .Kind "pointerMap")}}
    if update.{{.ProtoGoName}}Clear {
        value.{{.RuntimeName}}.Clear()
    }
    for key, fieldValue := range update.{{.ProtoGoName}}Set {
{{- if eq .Kind "pointerMap"}}
        child := new({{.ElementType}})
        if fieldValue != nil {
            {{.ChildCall "LoadSnapshot" "child" "fieldValue"}}
        }
        value.{{.RuntimeName}}.Store({{if ne .KeyType .ProtoKeyType}}{{.KeyType}}({{end}}key{{if ne .KeyType .ProtoKeyType}}){{end}}, child)
{{- else}}
        value.{{.RuntimeName}}.Store({{if ne .KeyType .ProtoKeyType}}{{.KeyType}}({{end}}key{{if ne .KeyType .ProtoKeyType}}){{end}}, {{.Decode "fieldValue"}})
{{- end}}
    }
    for _, key := range update.{{.ProtoGoName}}Delete {
        value.{{.RuntimeName}}.Delete({{if ne .KeyType .ProtoKeyType}}{{.KeyType}}({{end}}key{{if ne .KeyType .ProtoKeyType}}){{end}})
    }
{{- if eq .Kind "pointerMap"}}
    for key, childUpdate := range update.{{.ProtoGoName}}Update {
        child, _ := value.{{.RuntimeName}}.Load({{if ne .KeyType .ProtoKeyType}}{{.KeyType}}({{end}}key{{if ne .KeyType .ProtoKeyType}}){{end}})
        {{.ChildCall "Merge" "child" "childUpdate"}}
    }
{{- end}}
{{- else}}
    if update.{{.ProtoGoName}}Updated {
        value.{{.RuntimeName}}.Clear()
        for _, fieldValue := range update.{{.ProtoGoName}}Update {
{{- if eq .Kind "pointerSlice"}}
            var child {{.ValueType}}
            if fieldValue != nil {
                child = new({{.ElementType}})
                {{.ChildCall "LoadSnapshot" "child" "fieldValue"}}
            }
            value.{{.RuntimeName}}.Append(child)
{{- else}}
            value.{{.RuntimeName}}.Append({{.Decode "fieldValue"}})
{{- end}}
        }
    }
{{- end}}
{{end}}
}
{{end}}

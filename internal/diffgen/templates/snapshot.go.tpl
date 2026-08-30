{{define "snapshot"}}{{range .Types}}{{$type := .}}
func (value *{{.Receiver}}) Commit() {{.DiffAlias}}.Delta[*{{.Receiver}}] {
	return {{.DiffAlias}}.Delta[*{{.Receiver}}](value.Object.Commit())
}

func (value *{{.Receiver}}) Snapshot() []byte {
	return value.AppendDiffValue(nil)
}

func (value *{{.Receiver}}) LoadSnapshot(data []byte) error {
	value.InitLink(nil)
{{range .Fields}}{{if .Primitive}}	var zero{{.Name}} {{.ValueType}}
	value.{{.RuntimeName}}.SetValue(zero{{.Name}})
{{else if .Pointer}}	value.{{.RuntimeName}}.SetValue(nil)
{{else}}	value.{{.RuntimeName}}.Clear()
{{end}}{{end}}	fields, err := {{.DiffAlias}}.DecodeFields(data)
	if err != nil {
		return err
	}
	for _, field := range fields {
		if err := value.loadDiffField(field); err != nil {
			return err
		}
	}
	return nil
}

func (value *{{.Receiver}}) Merge(data []byte) error {
	value.InitLink(nil)
	patches, err := {{.DiffAlias}}.DecodePatches(data)
	if err != nil {
		return err
	}
	for _, patch := range patches {
		if err := value.MergeDiffPatch(patch.Path, patch.Operation, patch.Value); err != nil {
			return err
		}
	}
	return nil
}

func (value *{{.Receiver}}) loadDiffField(field {{.DiffAlias}}.EncodedField) error {
	switch field.FieldIndex {
{{range .Fields}}	case {{.DiffIndex}}:
		{
{{template "loadFieldValue" .}}		}
{{end}}	}
	return nil
}

func (value *{{.Receiver}}) MergeDiffPatch(path []{{.DiffAlias}}.EncodedPathNode, operation {{.DiffAlias}}.Operation, data []byte) error {
	if len(path) == 0 {
		return {{.DiffAlias}}.ErrInvalidData
	}
	node := path[0]
	switch node.FieldIndex {
{{range .Fields}}	case {{.DiffIndex}}:
		{
{{template "mergeField" .}}		}
{{end}}	}
	return nil
}
{{end}}{{end}}

{{define "loadFieldValue"}}{{if .Primitive}}			fieldValue, err := {{.DiffAlias}}.DecodePrimitive[{{.ValueType}}](field.Value)
			if err != nil {
				return err
			}
			value.{{.RuntimeName}}.SetValue(fieldValue)
{{else if .Pointer}}			fieldValue := new({{.ElementType}})
			if err := fieldValue.LoadSnapshot(field.Value); err != nil {
				return err
			}
			value.{{.RuntimeName}}.SetValue(fieldValue)
{{else if .Map}}			entries, err := {{.DiffAlias}}.DecodeValues(field.Value)
			if err != nil {
				return err
			}
			if len(entries)%2 != 0 {
				return {{.DiffAlias}}.ErrInvalidData
			}
			for index := 0; index < len(entries); index += 2 {
				mapKey, err := {{.DiffAlias}}.DecodePrimitive[{{.KeyType}}](entries[index])
				if err != nil {
					return err
				}
{{if .PrimitiveMap}}				mapValue, err := {{.DiffAlias}}.DecodePrimitive[{{.ValueType}}](entries[index+1])
				if err != nil {
					return err
				}
{{else}}				mapValue := new({{.ElementType}})
				if err := mapValue.LoadSnapshot(entries[index+1]); err != nil {
					return err
				}
{{end}}				value.{{.RuntimeName}}.Store(mapKey, mapValue)
			}
{{else if .Slice}}			elements, err := {{.DiffAlias}}.DecodeValues(field.Value)
			if err != nil {
				return err
			}
			for _, element := range elements {
{{if .PrimitiveSlice}}				sliceValue, err := {{.DiffAlias}}.DecodePrimitive[{{.ValueType}}](element)
				if err != nil {
					return err
				}
				value.{{.RuntimeName}}.Append(sliceValue)
{{else}}				elementData, exists, err := {{.DiffAlias}}.DecodePointerElement(element)
				if err != nil {
					return err
				}
				if !exists {
					value.{{.RuntimeName}}.Append(nil)
					continue
				}
				sliceValue := new({{.ElementType}})
				if err := sliceValue.LoadSnapshot(elementData); err != nil {
					return err
				}
				value.{{.RuntimeName}}.Append(sliceValue)
{{end}}			}
{{end}}			return nil
{{end}}

{{define "mergeField"}}{{if .Primitive}}			if node.KeyType != {{.DiffAlias}}.PathField || len(path) != 1 || operation != {{.DiffAlias}}.PrimitiveSet {
				return {{.DiffAlias}}.ErrInvalidData
			}
			fieldValue, err := {{.DiffAlias}}.DecodePrimitive[{{.ValueType}}](data)
			if err != nil {
				return err
			}
			value.{{.RuntimeName}}.SetValue(fieldValue)
			return nil
{{else if .Pointer}}			if node.KeyType != {{.DiffAlias}}.PathField {
				return {{.DiffAlias}}.ErrInvalidData
			}
			if len(path) == 1 {
				switch operation {
				case {{.DiffAlias}}.PointerSet:
					fieldValue := new({{.ElementType}})
					if err := fieldValue.LoadSnapshot(data); err != nil {
						return err
					}
					value.{{.RuntimeName}}.SetValue(fieldValue)
				case {{.DiffAlias}}.PointerClear:
					value.{{.RuntimeName}}.SetValue(nil)
				default:
					return {{.DiffAlias}}.ErrInvalidData
				}
				return nil
			}
			fieldValue := value.{{.RuntimeName}}.GetValue()
			if fieldValue == nil {
				return {{.DiffAlias}}.ErrInvalidData
			}
			return fieldValue.MergeDiffPatch(path[1:], operation, data)
{{else if .Map}}{{template "mergeMap" .}}{{else if .Slice}}			if node.KeyType != {{.DiffAlias}}.PathField || len(path) != 1 || operation != {{.DiffAlias}}.SliceReplace {
				return {{.DiffAlias}}.ErrInvalidData
			}
			value.{{.RuntimeName}}.Clear()
			field := {{.DiffAlias}}.EncodedField{FieldIndex: {{.DiffIndex}}, Value: data}
{{template "loadFieldValue" .}}{{end}}{{end}}

{{define "mergeMap"}}			if node.KeyType == {{.DiffAlias}}.PathField {
				if len(path) != 1 || operation != {{.DiffAlias}}.MapClear {
					return {{.DiffAlias}}.ErrInvalidData
				}
				value.{{.RuntimeName}}.Clear()
				return nil
			}
			if node.KeyType != {{.DiffAlias}}.PathMap {
				return {{.DiffAlias}}.ErrInvalidData
			}
			mapKey, err := {{.DiffAlias}}.DecodePrimitive[{{.KeyType}}](node.MapKey)
			if err != nil {
				return err
			}
{{if .PrimitiveMap}}			if len(path) != 1 {
				return {{.DiffAlias}}.ErrInvalidData
			}
			switch operation {
			case {{.DiffAlias}}.MapSet:
				mapValue, err := {{.DiffAlias}}.DecodePrimitive[{{.ValueType}}](data)
				if err != nil {
					return err
				}
				value.{{.RuntimeName}}.Store(mapKey, mapValue)
			case {{.DiffAlias}}.MapDelete:
				value.{{.RuntimeName}}.Delete(mapKey)
			default:
				return {{.DiffAlias}}.ErrInvalidData
			}
			return nil
{{else}}			if len(path) == 1 {
				switch operation {
				case {{.DiffAlias}}.MapSet:
					mapValue := new({{.ElementType}})
					if err := mapValue.LoadSnapshot(data); err != nil {
						return err
					}
					value.{{.RuntimeName}}.Store(mapKey, mapValue)
				case {{.DiffAlias}}.MapDelete:
					value.{{.RuntimeName}}.Delete(mapKey)
				default:
					return {{.DiffAlias}}.ErrInvalidData
				}
				return nil
			}
			mapValue, exists := value.{{.RuntimeName}}.Load(mapKey)
			if !exists {
				return {{.DiffAlias}}.ErrInvalidData
			}
			return mapValue.MergeDiffPatch(path[1:], operation, data)
{{end}}{{end}}

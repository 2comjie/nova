{{define "debug"}}{{range .Types}}{{$type := .}}
func (value *{{.Receiver}}) FormatDelta(data []byte) (string, error) {
	patches, err := {{.DiffAlias}}.DecodePatches(data)
	if err != nil {
		return "", err
	}
	debugPatches := make([]{{.DiffAlias}}.DebugPatch, 0, len(patches))
	for _, patch := range patches {
		path, patchValue, err := value.FormatDiffPatch(patch.Path, patch.Operation, patch.Value)
		if err != nil {
			return "", err
		}
		debugPatches = append(debugPatches, {{.DiffAlias}}.DebugPatch{
			Path: path,
			Operation: patch.Operation,
			Value: patchValue,
		})
	}
	return {{.DiffAlias}}.FormatDebugPatches("{{.Name}}", debugPatches), nil
}

func (value *{{.Receiver}}) FormatDiffPatch(path []{{.DiffAlias}}.EncodedPathNode, operation {{.DiffAlias}}.Operation, data []byte) (string, any, error) {
	if len(path) == 0 {
		return "", nil, {{.DiffAlias}}.ErrInvalidData
	}
	node := path[0]
	switch node.FieldIndex {
{{range .Fields}}	case {{.DiffIndex}}:
		{
{{template "debugField" .}}		}
{{end}}	}
	return "", nil, {{.DiffAlias}}.ErrInvalidData
}
{{end}}{{end}}

{{define "debugField"}}{{if .Primitive}}			if node.KeyType != {{.DiffAlias}}.PathField || len(path) != 1 || operation != {{.DiffAlias}}.PrimitiveSet {
				return "", nil, {{.DiffAlias}}.ErrInvalidData
			}
			fieldValue, err := {{.DiffAlias}}.DecodePrimitive[{{.ValueType}}](data)
			if err != nil {
				return "", nil, err
			}
			return "{{.Name}}", fieldValue, nil
{{else if .Pointer}}			if node.KeyType != {{.DiffAlias}}.PathField {
				return "", nil, {{.DiffAlias}}.ErrInvalidData
			}
			if len(path) == 1 {
				switch operation {
				case {{.DiffAlias}}.PointerSet:
					return "{{.Name}}", {{.DiffAlias}}.DebugSnapshot{Type: "{{.ValueType}}", Size: len(data)}, nil
				case {{.DiffAlias}}.PointerClear:
					return "{{.Name}}", nil, nil
				default:
					return "", nil, {{.DiffAlias}}.ErrInvalidData
				}
			}
			childPath, fieldValue, err := new({{.ElementType}}).FormatDiffPatch(path[1:], operation, data)
			if err != nil {
				return "", nil, err
			}
			return {{.DiffAlias}}.DebugFieldPath("{{.Name}}", childPath), fieldValue, nil
{{else if .Map}}{{template "debugMap" .}}{{else if .Slice}}{{template "debugSlice" .}}{{end}}{{end}}

{{define "debugMap"}}			if node.KeyType == {{.DiffAlias}}.PathField {
				if len(path) != 1 || operation != {{.DiffAlias}}.MapClear {
					return "", nil, {{.DiffAlias}}.ErrInvalidData
				}
				return "{{.Name}}", nil, nil
			}
			if node.KeyType != {{.DiffAlias}}.PathMap {
				return "", nil, {{.DiffAlias}}.ErrInvalidData
			}
			mapKey, err := {{.DiffAlias}}.DecodePrimitive[{{.KeyType}}](node.MapKey)
			if err != nil {
				return "", nil, err
			}
{{if .PrimitiveMap}}			if len(path) != 1 {
				return "", nil, {{.DiffAlias}}.ErrInvalidData
			}
			switch operation {
			case {{.DiffAlias}}.MapSet:
				mapValue, err := {{.DiffAlias}}.DecodePrimitive[{{.ValueType}}](data)
				if err != nil {
					return "", nil, err
				}
				return {{.DiffAlias}}.DebugMapPath("{{.Name}}", mapKey, ""), mapValue, nil
			case {{.DiffAlias}}.MapDelete:
				return {{.DiffAlias}}.DebugMapPath("{{.Name}}", mapKey, ""), nil, nil
			default:
				return "", nil, {{.DiffAlias}}.ErrInvalidData
			}
{{else}}			if len(path) == 1 {
				switch operation {
				case {{.DiffAlias}}.MapSet:
					return {{.DiffAlias}}.DebugMapPath("{{.Name}}", mapKey, ""), {{.DiffAlias}}.DebugSnapshot{Type: "{{.ValueType}}", Size: len(data)}, nil
				case {{.DiffAlias}}.MapDelete:
					return {{.DiffAlias}}.DebugMapPath("{{.Name}}", mapKey, ""), nil, nil
				default:
					return "", nil, {{.DiffAlias}}.ErrInvalidData
				}
			}
			childPath, mapValue, err := new({{.ElementType}}).FormatDiffPatch(path[1:], operation, data)
			if err != nil {
				return "", nil, err
			}
			return {{.DiffAlias}}.DebugMapPath("{{.Name}}", mapKey, childPath), mapValue, nil
{{end}}{{end}}

{{define "debugSlice"}}			if node.KeyType != {{.DiffAlias}}.PathField || len(path) != 1 || operation != {{.DiffAlias}}.SliceReplace {
				return "", nil, {{.DiffAlias}}.ErrInvalidData
			}
			elements, err := {{.DiffAlias}}.DecodeValues(data)
			if err != nil {
				return "", nil, err
			}
{{if .PrimitiveSlice}}			values := make([]{{.ValueType}}, 0, len(elements))
			for _, element := range elements {
				fieldValue, err := {{.DiffAlias}}.DecodePrimitive[{{.ValueType}}](element)
				if err != nil {
					return "", nil, err
				}
				values = append(values, fieldValue)
			}
			return "{{.Name}}", values, nil
{{else}}			values := make([]any, 0, len(elements))
			for _, element := range elements {
				elementData, exists, err := {{.DiffAlias}}.DecodePointerElement(element)
				if err != nil {
					return "", nil, err
				}
				if !exists {
					values = append(values, nil)
					continue
				}
				values = append(values, {{.DiffAlias}}.DebugSnapshot{Type: "{{.ValueType}}", Size: len(elementData)})
			}
			return "{{.Name}}", values, nil
{{end}}{{end}}

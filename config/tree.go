package config

import (
	"fmt"
	"path"
	"reflect"
	"sort"
	"strings"

	"github.com/2comjie/nova/encoding"
)

func buildTree(sources []Source) (map[string]any, error) {
	root := make(map[string]any)
	for sourceIndex, source := range sources {
		if source == nil {
			return nil, fmt.Errorf("config: source %d is nil", sourceIndex)
		}
		documents, err := source.Load()
		if err != nil {
			return nil, fmt.Errorf("config: load source %d: %w", sourceIndex, err)
		}
		sort.Slice(documents, func(i, j int) bool {
			return documents[i].Path < documents[j].Path
		})

		seen := make(map[string]string, len(documents))
		for _, document := range documents {
			segments, format, err := documentInfo(document)
			if err != nil {
				return nil, err
			}
			logicalPath := strings.Join(segments, ".")
			if previous, ok := seen[logicalPath]; ok {
				return nil, fmt.Errorf(
					"config: source %d 中的 %q 与 %q 映射到同一配置路径 %q",
					sourceIndex,
					previous,
					document.Path,
					logicalPath,
				)
			}
			seen[logicalPath] = document.Path

			value, err := decodeDocument(document, format, segments)
			if err != nil {
				return nil, err
			}
			if err := mergeDocument(root, segments, value); err != nil {
				return nil, fmt.Errorf("config: merge %q: %w", document.Path, err)
			}
		}
	}
	return root, nil
}

func documentInfo(document Document) ([]string, string, error) {
	format := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(document.Format), "."))
	rawPath := strings.ReplaceAll(document.Path, "\\", "/")
	if rawPath == "" {
		if format == "" {
			return nil, "", fmt.Errorf("config: 根配置文档缺少格式")
		}
		return nil, format, nil
	}

	cleanPath := path.Clean(rawPath)
	if path.IsAbs(cleanPath) || cleanPath == ".." || strings.HasPrefix(cleanPath, "../") {
		return nil, "", fmt.Errorf("config: 非法文档路径 %q", document.Path)
	}
	extension := path.Ext(cleanPath)
	if format == "" {
		format = strings.ToLower(strings.TrimPrefix(extension, "."))
	}
	if format == "" {
		return nil, "", fmt.Errorf("config: 文档 %q 缺少格式", document.Path)
	}
	if extension != "" {
		cleanPath = strings.TrimSuffix(cleanPath, extension)
	}

	var segments []string
	for _, component := range strings.Split(cleanPath, "/") {
		for _, segment := range strings.Split(component, ".") {
			if segment == "" {
				return nil, "", fmt.Errorf("config: 非法文档路径 %q", document.Path)
			}
			segments = append(segments, segment)
		}
	}
	return segments, format, nil
}

func decodeDocument(document Document, format string, segments []string) (any, error) {
	codec := encoding.GetCodec(format)
	if codec == nil {
		return nil, fmt.Errorf("config: 文档 %q 使用了不支持的格式 %q", document.Path, format)
	}

	var decoded any
	if err := codec.Unmarshal(document.Data, &decoded); err != nil {
		return nil, fmt.Errorf("config: 解析文档 %q: %w", document.Path, err)
	}
	decoded = normalize(decoded)

	// XML 必须有根元素。根元素与文件名一致时，文件路径已经表达了这一层，避免重复嵌套。
	if format == "xml" && len(segments) > 0 {
		if object, ok := decoded.(map[string]any); ok && len(object) == 1 {
			if value, exists := object[segments[len(segments)-1]]; exists {
				decoded = value
			}
		}
	}
	return decoded, nil
}

func mergeDocument(root map[string]any, segments []string, value any) error {
	if len(segments) == 0 {
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("根配置文档必须是对象，实际为 %T", value)
		}
		mergeMap(root, object)
		return nil
	}

	current := root
	for _, segment := range segments[:len(segments)-1] {
		next, exists := current[segment]
		if !exists {
			child := make(map[string]any)
			current[segment] = child
			current = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("路径 %q 已经是 %T，不能作为目录", segment, next)
		}
		current = child
	}

	leaf := segments[len(segments)-1]
	if existing, exists := current[leaf]; exists {
		existingMap, existingOK := existing.(map[string]any)
		incomingMap, incomingOK := value.(map[string]any)
		if existingOK && incomingOK {
			mergeMap(existingMap, incomingMap)
			return nil
		}
	}
	current[leaf] = clone(value)
	return nil
}

func mergeMap(destination, source map[string]any) {
	for key, incoming := range source {
		if incomingMap, ok := incoming.(map[string]any); ok {
			if existingMap, ok := destination[key].(map[string]any); ok {
				mergeMap(existingMap, incomingMap)
				continue
			}
		}
		destination[key] = clone(incoming)
	}
}

func readTree(root map[string]any, key string) (any, bool) {
	if key == "" {
		return root, true
	}
	var current any = root
	for _, segment := range strings.Split(key, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func normalize(value any) any {
	if value == nil {
		return nil
	}
	rv := reflect.ValueOf(value)
	for rv.Kind() == reflect.Interface || rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Map:
		result := make(map[string]any, rv.Len())
		iterator := rv.MapRange()
		for iterator.Next() {
			result[fmt.Sprint(iterator.Key().Interface())] = normalize(iterator.Value().Interface())
		}
		return result
	case reflect.Slice, reflect.Array:
		if bytes, ok := value.([]byte); ok {
			return string(bytes)
		}
		result := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			result[i] = normalize(rv.Index(i).Interface())
		}
		return result
	default:
		return rv.Interface()
	}
}

func clone(value any) any {
	switch current := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(current))
		for key, child := range current {
			result[key] = clone(child)
		}
		return result
	case []any:
		result := make([]any, len(current))
		for i, child := range current {
			result[i] = clone(child)
		}
		return result
	case []byte:
		return append([]byte(nil), current...)
	default:
		return current
	}
}

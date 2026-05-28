package config

import "fmt"

func walk(v any, fn func(any) any) any {
	switch val := v.(type) {
	case map[string]any:
		dst := make(map[string]any, len(val))
		for k, elem := range val {
			dst[k] = walk(elem, fn)
		}
		return fn(dst)
	case map[any]any:
		dst := make(map[string]any, len(val))
		for k, elem := range val {
			dst[fmt.Sprint(k)] = walk(elem, fn)
		}
		return fn(dst)
	case []any:
		dst := make([]any, len(val))
		for i, elem := range val {
			dst[i] = walk(elem, fn)
		}
		return fn(dst)
	default:
		return fn(val)
	}
}

func clone(v any) any {
	return walk(v, func(x any) any { return x })
}

func normalize(v any) any {
	return walk(v, func(x any) any {
		if b, ok := x.([]byte); ok {
			return string(b)
		}
		return x
	})
}

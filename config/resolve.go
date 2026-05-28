package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var placeholderRe = regexp.MustCompile(`\${([^}]*)}`)

func resolve(tree map[string]any, toType bool) {
	var resolveInPlace func(any)
	resolveInPlace = func(v any) {
		switch val := v.(type) {
		case map[string]any:
			for k, elem := range val {
				if s, ok := elem.(string); ok {
					val[k] = expandString(s, tree, toType)
				} else {
					resolveInPlace(elem)
				}
			}
		case []any:
			for i, elem := range val {
				if s, ok := elem.(string); ok {
					val[i] = expandString(s, tree, toType)
				} else {
					resolveInPlace(elem)
				}
			}
		}
	}
	resolveInPlace(tree)
}

func expandString(s string, tree map[string]any, toType bool) any {
	matches := placeholderRe.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return s
	}
	if toType && len(matches) == 1 && matches[0][0] == s {
		if v := lookupString(tree, matches[0][1]); v != "" {
			return convertToType(v)
		}
	}
	for _, m := range matches {
		s = strings.ReplaceAll(s, m[0], lookupString(tree, m[1]))
	}
	return s
}

func lookupString(tree map[string]any, expr string) string {
	args := strings.SplitN(strings.TrimSpace(expr), ":", 2)
	v, ok := readTree(tree, args[0])
	if !ok {
		if len(args) > 1 {
			return args[1]
		}
		return ""
	}
	s := fmt.Sprint(v)
	if s == "" {
		if len(args) > 1 {
			return args[1]
		}
	}
	return s
}

func readTree(tree map[string]any, path string) (any, bool) {
	keys := strings.Split(path, ".")
	var cur any = tree
	for _, key := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := m[key]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

const (
	boolTrue  = "true"
	boolFalse = "false"
)

func convertToType(s string) any {
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		return strings.Trim(s, `"`)
	}
	if s == boolTrue || s == boolFalse {
		b, _ := strconv.ParseBool(s)
		return b
	}
	if strings.Contains(s, ".") {
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	return s
}

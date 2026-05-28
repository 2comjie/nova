package config

func merge(dst, src map[string]any) {
	for k, sv := range src {
		if srcMap, ok := sv.(map[string]any); ok {
			if dstMap, ok := dst[k].(map[string]any); ok {
				merge(dstMap, srcMap)
				continue
			}
		}
		dst[k] = clone(sv)
	}
}

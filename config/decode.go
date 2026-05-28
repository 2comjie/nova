package config

import (
	"fmt"
	"strings"

	"github.com/2comjie/wali/encoding"
)

type Decoder func(*KeyValue, map[string]any) error

func defaultDecoder(src *KeyValue, target map[string]any) error {
	if src.Format == "" {
		keys := strings.Split(src.Key, ".")
		for i, k := range keys {
			if i == len(keys)-1 {
				target[k] = src.Value
				return nil
			}
			if target[k] == nil {
				sub := make(map[string]any)
				target[k] = sub
				target = sub
			} else if sub, ok := target[k].(map[string]any); ok {
				target = sub
			} else {
				sub := make(map[string]any)
				target[k] = sub
				target = sub
			}
		}
		return nil
	}
	if codec := encoding.GetCodec(src.Format); codec != nil {
		return codec.Unmarshal(src.Value, &target)
	}
	return fmt.Errorf("config: unsupported format %q for key %q", src.Format, src.Key)
}

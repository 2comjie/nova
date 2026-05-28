package json

import (
	"encoding/json"

	"github.com/2comjie/wali/encoding"
)

func init() {
	encoding.RegisterCodec(&jsonCodec{})
}

type jsonCodec struct{}

func (c *jsonCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (c *jsonCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func (c *jsonCodec) Name() string {
	return "json"
}

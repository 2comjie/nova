package json

import (
	"encoding/json"
)

type JsonCodec struct{}

func (c *JsonCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func (c *JsonCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func (c *JsonCodec) Name() string {
	return "json"
}

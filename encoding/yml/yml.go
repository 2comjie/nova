package yml

import (
	"gopkg.in/yaml.v3"
)

type YamlCodec struct{}

func (c *YamlCodec) Marshal(v any) ([]byte, error) {
	return yaml.Marshal(v)
}

func (c *YamlCodec) Unmarshal(data []byte, v any) error {
	return yaml.Unmarshal(data, v)
}

func (c *YamlCodec) Name() string {
	return "yaml"
}

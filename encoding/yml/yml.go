package yml

import (
	"github.com/2comjie/wali/encoding"
	"gopkg.in/yaml.v3"
)

func init() {
	encoding.RegisterCodec(&yamlCodec{})
}

type yamlCodec struct{}

func (c *yamlCodec) Marshal(v any) ([]byte, error) {
	return yaml.Marshal(v)
}

func (c *yamlCodec) Unmarshal(data []byte, v any) error {
	return yaml.Unmarshal(data, v)
}

func (c *yamlCodec) Name() string {
	return "yaml"
}

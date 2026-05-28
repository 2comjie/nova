package xml

import (
	"encoding/xml"

	"github.com/2comjie/wali/encoding"
)

func init() {
	encoding.RegisterCodec(&xmlCodec{})
}

type xmlCodec struct{}

func (c *xmlCodec) Marshal(v any) ([]byte, error) {
	return xml.Marshal(v)
}

func (c *xmlCodec) Unmarshal(data []byte, v any) error {
	return xml.Unmarshal(data, v)
}

func (c *xmlCodec) Name() string {
	return "xml"
}

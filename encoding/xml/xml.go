package xml

import (
	"encoding/xml"
)

type XmlCodec struct{}

func (c *XmlCodec) Marshal(v any) ([]byte, error) {
	return xml.Marshal(v)
}

func (c *XmlCodec) Unmarshal(data []byte, v any) error {
	return xml.Unmarshal(data, v)
}

func (c *XmlCodec) Name() string {
	return "xml"
}

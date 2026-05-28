package xml

import (
	"encoding/xml"
	"reflect"

	"github.com/clbanning/mxj"
)

type XmlCodec struct{}

func (c *XmlCodec) Marshal(v any) ([]byte, error) {
	return xml.Marshal(v)
}

func (c *XmlCodec) Unmarshal(data []byte, v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Map || rv.Kind() == reflect.Interface {
		m, err := mxj.NewMapXml(data)
		if err != nil {
			return err
		}
		rv.Set(reflect.ValueOf(m))
		return nil
	}
	return xml.Unmarshal(data, v)
}

func (c *XmlCodec) Name() string {
	return "xml"
}

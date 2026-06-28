package encoding

import (
	"strings"

	"github.com/2comjie/wali/encoding/gob"
	"github.com/2comjie/wali/encoding/json"
	"github.com/2comjie/wali/encoding/proto"
	"github.com/2comjie/wali/encoding/xml"
	"github.com/2comjie/wali/encoding/yml"
)

func init() {
	RegisterCodec(&json.JsonCodec{})
	RegisterCodec(&proto.ProtoCodec{})
	RegisterCodec(&xml.XmlCodec{})
	RegisterCodec(&gob.GobCodec{})
	// yaml also maps to "yml" for .yml file extension
	registeredCodecs["yml"] = &yml.YamlCodec{}
	registeredCodecs["yaml"] = &yml.YamlCodec{}
}

type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
	Name() string
}

var registeredCodecs = make(map[string]Codec)

func RegisterCodec(codec Codec) {
	if codec == nil {
		panic("cannot register a nil Codec")
	}
	if codec.Name() == "" {
		panic("cannot register Codec with empty string result for Name()")
	}
	contentSubtype := strings.ToLower(codec.Name())
	registeredCodecs[contentSubtype] = codec
}

func GetCodec(contentSubtype string) Codec {
	return registeredCodecs[contentSubtype]
}

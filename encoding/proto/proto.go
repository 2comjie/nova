package proto

import (
	"github.com/2comjie/wali/encoding"
	"google.golang.org/protobuf/proto"
)

func init() {
	encoding.RegisterCodec(&protoCodec{})
}

type protoCodec struct{}

func (c *protoCodec) Marshal(v any) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}

func (c *protoCodec) Unmarshal(data []byte, v any) error {
	return proto.Unmarshal(data, v.(proto.Message))
}

func (c *protoCodec) Name() string {
	return "proto"
}

package proto

import (
	"google.golang.org/protobuf/proto"
)

type ProtoCodec struct{}

func (c *ProtoCodec) Marshal(v any) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}

func (c *ProtoCodec) Unmarshal(data []byte, v any) error {
	return proto.Unmarshal(data, v.(proto.Message))
}

func (c *ProtoCodec) Name() string {
	return "proto"
}

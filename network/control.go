package network

import (
	"github.com/2comjie/wali/core/buffer"
	"google.golang.org/protobuf/proto"
)

func marshalControl(message proto.Message) ([]byte, *buffer.Bytes, error) {
	holder := buffer.MallocBytes(proto.Size(message))
	body, err := proto.MarshalOptions{}.MarshalAppend(holder.Bytes()[:0], message)
	if err != nil {
		holder.Release()
		return nil, nil, err
	}
	return body, holder, nil
}

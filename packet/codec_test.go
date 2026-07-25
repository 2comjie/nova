package packet

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestCodecGolden(t *testing.T) {
	codec := NewCodec(DefaultMaxFrame)
	message := &Message{
		Type:  Req,
		Route: 7,
		Seq:   9,
		Body:  []byte{1, 2, 3},
	}

	frame, err := codec.Encode(message)
	if err != nil {
		t.Fatal(err)
	}
	defer frame.Release()

	const want = "822f000100000017000000070000000000000009010203"
	if got := hex.EncodeToString(frame.Bytes()); got != want {
		t.Fatalf("编码结果不一致: got=%s want=%s", got, want)
	}

	decoded, err := codec.Read(bytes.NewReader(frame.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer decoded.Release()
	if decoded.Type != message.Type || decoded.Route != message.Route || decoded.Seq != message.Seq ||
		!bytes.Equal(decoded.Body, message.Body) {
		t.Fatalf("解码结果不一致: %#v", decoded)
	}
}

func TestCodecRejectInvalidMessage(t *testing.T) {
	codec := NewCodec(DefaultMaxFrame)
	tests := []*Message{
		{Type: Req, Route: 0, Seq: 1},
		{Type: Rsp, Route: 1, Seq: 0},
		{Type: Push, Route: 1, Seq: 1},
		{Type: Ping, Body: []byte{1}},
		{Type: 99},
	}
	for _, message := range tests {
		if _, err := codec.Encode(message); err == nil {
			t.Fatalf("非法包未被拒绝: %#v", message)
		}
	}
}

func TestCodecAllTypes(t *testing.T) {
	codec := NewCodec(DefaultMaxFrame)
	messages := []*Message{
		{Type: Req, Route: 1, Seq: 1, Body: []byte("req")},
		{Type: Req, Route: 2, Body: []byte("tell")},
		{Type: Rsp, Route: 1, Seq: 1, Body: []byte("rsp")},
		{Type: Push, Route: 2, Body: []byte("push")},
		{Type: Ping, Body: make([]byte, PingBodySize)},
		{Type: Pong, Body: make([]byte, PingBodySize)},
		{Type: BindReq, Body: []byte("bind-req")},
		{Type: BindRsp, Body: []byte("bind-rsp")},
	}
	for _, message := range messages {
		frame, err := codec.Encode(message)
		if err != nil {
			t.Fatalf("编码%v失败: %v", message.Type, err)
		}
		decoded, err := codec.Read(bytes.NewReader(frame.Bytes()))
		frame.Release()
		if err != nil {
			t.Fatalf("解码%v失败: %v", message.Type, err)
		}
		if decoded.Type != message.Type || decoded.Route != message.Route ||
			decoded.Seq != message.Seq || !bytes.Equal(decoded.Body, message.Body) {
			decoded.Release()
			t.Fatalf("包%v往返不一致", message.Type)
		}
		decoded.Release()
	}
}

func FuzzCodecRead(f *testing.F) {
	f.Add([]byte{0x57, 0x4c, 0, 4, 0, 0, 0, 28, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8})
	f.Add([]byte{0x57, 0x4c})
	f.Fuzz(func(t *testing.T, data []byte) {
		codec := NewCodec(1024)
		message, _ := codec.Read(bytes.NewReader(data))
		if message != nil {
			message.Release()
		}
	})
}

package rpc

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestErrorEncodeDecode(t *testing.T) {
	err := DecodeError(EncodeError(NewErrorWithDetail(10001, "coin not enough", []byte("player-2"))))
	var rpcError *Error
	if !errors.As(err, &rpcError) {
		t.Fatalf("error=%v", err)
	}
	if rpcError.Code != 10001 || rpcError.Message != "coin not enough" || string(rpcError.Detail) != "player-2" {
		t.Fatalf("rpc error=%+v", rpcError)
	}
}

func TestGRPCErrorPassesThrough(t *testing.T) {
	err := status.Error(codes.Unavailable, "service unavailable")
	decoded := DecodeError(err)
	if status.Code(decoded) != codes.Unavailable {
		t.Fatalf("error=%v", decoded)
	}
}

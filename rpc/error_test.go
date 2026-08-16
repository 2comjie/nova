package rpc_test

import (
	"context"
	"testing"

	"github.com/2comjie/wali/rpc"
	"github.com/2comjie/wali/rpc/rpcerr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBusinessError(t *testing.T) {
	err := rpc.DecodeErr(rpc.EncodeError(rpcerr.NewWithDetail(10001, "coin not enough", []byte("player-1"))))
	if err.Code() != 10001 {
		t.Fatalf("code = %d", err.Code())
	}
	if err.Message() != "coin not enough" {
		t.Fatalf("message = %q", err.Message())
	}
	if string(err.Detail()) != "player-1" {
		t.Fatalf("detail = %q", err.Detail())
	}
	if err.GRPCCode() != codes.OK {
		t.Fatalf("grpc code = %s", err.GRPCCode())
	}
}

func TestTransportError(t *testing.T) {
	err := rpc.DecodeErr(rpc.EncodeError(rpcerr.NewGRPC(codes.Unavailable, "gate unavailable")))
	if err.Code() != 0 {
		t.Fatalf("code = %d", err.Code())
	}
	if err.Message() != "gate unavailable" {
		t.Fatalf("message = %q", err.Message())
	}
	if err.GRPCCode() != codes.Unavailable {
		t.Fatalf("grpc code = %s", err.GRPCCode())
	}
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("status code = %s", status.Code(err))
	}
}

func TestContextError(t *testing.T) {
	err := rpcerr.Wrap(context.DeadlineExceeded)
	if err.GRPCCode() != codes.DeadlineExceeded {
		t.Fatalf("grpc code = %s", err.GRPCCode())
	}
}

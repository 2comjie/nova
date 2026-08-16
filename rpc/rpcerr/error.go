package rpcerr

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Err interface {
	error
	Code() uint32
	Message() string
	Detail() []byte
	GRPCCode() codes.Code
}

type rpcError struct {
	code       uint32
	message    string
	detail     []byte
	grpcCode   codes.Code
	grpcStatus *status.Status
	cause      error
}

func New(code uint32, message string) Err {
	return &rpcError{code: code, message: message, grpcCode: codes.OK}
}

func NewWithDetail(code uint32, message string, detail []byte) Err {
	return &rpcError{code: code, message: message, detail: detail, grpcCode: codes.OK}
}

func Wrap(err error) Err {
	if err == nil {
		return nil
	}
	if rpcErr, ok := err.(Err); ok {
		return rpcErr
	}
	grpcStatus, ok := status.FromError(err)
	if !ok {
		grpcStatus = status.FromContextError(err)
	}
	return &rpcError{message: grpcStatus.Message(), grpcCode: grpcStatus.Code(), grpcStatus: grpcStatus, cause: err}
}

func (e *rpcError) Error() string {
	return e.message
}

func (e *rpcError) Code() uint32 {
	return e.code
}

func (e *rpcError) Message() string {
	return e.message
}

func (e *rpcError) Detail() []byte {
	return e.detail
}

func (e *rpcError) GRPCCode() codes.Code {
	return e.grpcCode
}

func (e *rpcError) ErrorCode() uint32 {
	return e.code
}

func (e *rpcError) ErrorDetail() []byte {
	return e.detail
}

func (e *rpcError) Unwrap() error {
	return e.cause
}

func (e *rpcError) GRPCStatus() *status.Status {
	if e.grpcStatus != nil {
		return e.grpcStatus
	}
	return status.New(codes.Unknown, e.message)
}

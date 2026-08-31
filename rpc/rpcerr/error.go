package rpcerr

import (
	"errors"

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

type codedError interface {
	ErrorCode() uint32
}

type detailError interface {
	ErrorDetail() []byte
}

type rpcError struct {
	code     uint32
	message  string
	detail   []byte
	grpcCode codes.Code
}

type businessError struct {
	*rpcError
}

type transportError struct {
	*rpcError
	grpcStatus *status.Status
}

func New(code uint32, message string) Err {
	return &businessError{rpcError: &rpcError{code: code, message: message, grpcCode: codes.OK}}
}

func NewWithDetail(code uint32, message string, detail []byte) Err {
	return &businessError{rpcError: &rpcError{code: code, message: message, detail: detail, grpcCode: codes.OK}}
}

func NewGRPC(code codes.Code, message string) Err {
	return Wrap(status.Error(code, message))
}

func Wrap(err error) Err {
	if err == nil {
		return nil
	}
	var rpcErr Err
	if errors.As(err, &rpcErr) {
		return rpcErr
	}
	var coded codedError
	if errors.As(err, &coded) {
		var detail []byte
		var detailed detailError
		if errors.As(err, &detailed) {
			detail = detailed.ErrorDetail()
		}
		return NewWithDetail(coded.ErrorCode(), err.Error(), detail)
	}
	grpcStatus, ok := status.FromError(err)
	if !ok {
		grpcStatus = status.FromContextError(err)
	}
	return &transportError{
		rpcError:   &rpcError{message: grpcStatus.Message(), grpcCode: grpcStatus.Code()},
		grpcStatus: grpcStatus,
	}
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

func (e *businessError) ErrorCode() uint32 {
	return e.code
}

func (e *businessError) ErrorDetail() []byte {
	return e.detail
}

func (e *transportError) GRPCStatus() *status.Status {
	return e.grpcStatus
}

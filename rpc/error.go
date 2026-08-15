package rpc

import (
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const ErrorCodeRedirect uint32 = 5

type CodedError interface {
	error
	ErrorCode() uint32
}

type DetailError interface {
	CodedError
	ErrorDetail() []byte
}

type Error struct {
	Code    uint32
	Message string
	Detail  []byte
}

func NewError(code uint32, message string) *Error {
	return &Error{Code: code, Message: message}
}

func NewErrorWithDetail(code uint32, message string, detail []byte) *Error {
	return &Error{Code: code, Message: message, Detail: detail}
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) ErrorCode() uint32 {
	return e.Code
}

func (e *Error) ErrorDetail() []byte {
	return e.Detail
}

func EncodeError(err error) error {
	if err == nil {
		return nil
	}

	var codedError CodedError
	if !errors.As(err, &codedError) {
		return err
	}

	var detail []byte
	var detailError DetailError
	if errors.As(err, &detailError) {
		detail = detailError.ErrorDetail()
	}

	encoded, encodeErr := status.New(codes.Unknown, err.Error()).WithDetails(&ErrorDetail{
		Code:   codedError.ErrorCode(),
		Detail: detail,
	})
	if encodeErr != nil {
		panic(encodeErr)
	}
	return encoded.Err()
}

func DecodeError(err error) error {
	if err == nil {
		return nil
	}

	statusValue, ok := status.FromError(err)
	if !ok {
		return err
	}
	for _, detail := range statusValue.Details() {
		if errorDetail, ok := detail.(*ErrorDetail); ok {
			return &Error{Code: errorDetail.Code, Message: statusValue.Message(), Detail: errorDetail.Detail}
		}
	}
	return err
}

type clientStream struct {
	grpc.ClientStream
}

func WrapClientStream(stream grpc.ClientStream) grpc.ClientStream {
	return &clientStream{ClientStream: stream}
}

func (s *clientStream) Header() (metadata.MD, error) {
	header, err := s.ClientStream.Header()
	return header, DecodeError(err)
}

func (s *clientStream) CloseSend() error {
	return DecodeError(s.ClientStream.CloseSend())
}

func (s *clientStream) SendMsg(message any) error {
	return DecodeError(s.ClientStream.SendMsg(message))
}

func (s *clientStream) RecvMsg(message any) error {
	return DecodeError(s.ClientStream.RecvMsg(message))
}

package rpc

import (
	"context"
	"errors"
)

// 框架错误使用高位区间，业务错误码独立定义。
const (
	CodeInternal         uint32 = 0xffff0001
	CodeInvalidArgument  uint32 = 0xffff0002
	CodeNotFound         uint32 = 0xffff0003
	CodeCanceled         uint32 = 0xffff0005
	CodeDeadlineExceeded uint32 = 0xffff0006
	CodeBusy             uint32 = 0xffff0007
	ErrorCodeRedirect    uint32 = 5
)

var (
	ErrClosed = errors.New("rpc: connection closed")
	ErrBusy   = NewError(CodeBusy, "rpc: too many pending requests")
)

func NewError(code uint32, message string) *Error {
	return &Error{Code: code, Message: message}
}

func NewErrorWithDetail(code uint32, message string, detail []byte) *Error {
	return &Error{Code: code, Message: message, Detail: detail}
}

func (e *Error) Error() string { return e.Message }

func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	if failure, ok := err.(*Error); ok {
		return failure
	}
	code := CodeInternal
	switch {
	case errors.Is(err, context.Canceled):
		code = CodeCanceled
	case errors.Is(err, context.DeadlineExceeded):
		code = CodeDeadlineExceeded
	}
	return NewError(code, err.Error())
}

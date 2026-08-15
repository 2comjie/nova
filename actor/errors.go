package actor

import "errors"

var (
	ErrActorNotActive          = errors.New("actor not active")
	ErrInvalidActivationPolicy = errors.New("invalid actor activation policy")
	ErrMessageHandlerPanic     = errors.New("actor message handler panic")
)

const (
	ErrorCodeActorNotActive          uint32 = 1
	ErrorCodeInvalidActivationPolicy uint32 = 2
	ErrorCodeSystemStopped           uint32 = 3
	ErrorCodeExecutionFailed         uint32 = 4
)

type CallError struct {
	Code    uint32
	Message string
}

func (e *CallError) Error() string {
	return e.Message
}

func (e *CallError) ActorErrorCode() uint32 {
	return e.Code
}

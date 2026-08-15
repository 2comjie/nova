package actor

import "github.com/2comjie/wali/rpc"

const (
	ErrorCodeActorNotActive          uint32 = 1
	ErrorCodeInvalidActivationPolicy uint32 = 2
	ErrorCodeSystemStopped           uint32 = 3
	ErrorCodeExecutionFailed         uint32 = 4
	ErrorCodeActorRedirect           uint32 = rpc.ErrorCodeRedirect
)

var (
	ErrActorNotActive          = rpc.NewError(ErrorCodeActorNotActive, "actor not active")
	ErrInvalidActivationPolicy = rpc.NewError(ErrorCodeInvalidActivationPolicy, "invalid actor activation policy")
	ErrSystemStopped           = rpc.NewError(ErrorCodeSystemStopped, "actor system stopped")
	ErrMessageHandlerPanic     = rpc.NewError(ErrorCodeExecutionFailed, "actor message handler panic")
)

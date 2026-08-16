package actor

import (
	"github.com/2comjie/nova/rpc"
	"github.com/2comjie/nova/rpc/rpcerr"
)

const (
	ErrorCodeActorNotActive          uint32 = 1
	ErrorCodeInvalidActivationPolicy uint32 = 2
	ErrorCodeSystemStopped           uint32 = 3
	ErrorCodeExecutionFailed         uint32 = 4
	ErrorCodeActorRedirect           uint32 = rpc.ErrorCodeRedirect
)

var (
	ErrActorNotActive          = rpcerr.New(ErrorCodeActorNotActive, "actor not active")
	ErrInvalidActivationPolicy = rpcerr.New(ErrorCodeInvalidActivationPolicy, "invalid actor activation policy")
	ErrSystemStopped           = rpcerr.New(ErrorCodeSystemStopped, "actor system stopped")
	ErrMessageHandlerPanic     = rpcerr.New(ErrorCodeExecutionFailed, "actor message handler panic")
)

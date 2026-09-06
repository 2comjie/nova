package actor

import "github.com/2comjie/nova/rpc"

const (
	ErrorCodeActorNotActive          uint32 = 1
	ErrorCodeInvalidActivationPolicy uint32 = 2
	ErrorCodeSystemStopped           uint32 = 3
	ErrorCodeActorRedirect           uint32 = rpc.ErrorCodeRedirect
)

var (
	ErrActorNotActive          = rpc.NewError(ErrorCodeActorNotActive, "actor not active")
	ErrInvalidActivationPolicy = rpc.NewError(ErrorCodeInvalidActivationPolicy, "invalid actor activation policy")
	ErrSystemStopped           = rpc.NewError(ErrorCodeSystemStopped, "actor system stopped")
)

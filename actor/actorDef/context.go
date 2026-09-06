package actorDef

import (
	"context"
	"time"
)

type ActorStartCtx struct {
	context.Context
	Self   Pid
	Unload func()
}

type ActorStopCtx struct {
	// Normal unload/shutdown keeps this context alive through OnStop. Lease loss
	// cancels it; persistence still needs ownership checks to fence stale writes.
	context.Context
	Self   Pid
	Reason StopReason
}

type ActorUpdateCtx struct {
	context.Context
	Self   Pid
	Delta  time.Duration
	Idle   time.Duration
	Unload func()
}
type StopReason uint8

const (
	StopReasonShutdown  StopReason = 1
	StopReasonUnload    StopReason = 2
	StopReasonLeaseLost StopReason = 3
)

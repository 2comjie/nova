package actorDef

import (
	"context"
	"time"
)

type ActorStartCtx struct {
	context.Context
	Self PID
}

type ActorStopCtx struct {
	context.Context
	Self   PID
	Reason StopReason
}

type ActorUpdateCtx struct {
	context.Context
	Self  PID
	Delta time.Duration
	Idle  time.Duration
}
type StopReason uint8

const (
	StopReasonShutdown  StopReason = 1
	StopReasonUnload    StopReason = 2
	StopReasonLeaseLost StopReason = 3
	StopReasonPanic     StopReason = 4
)

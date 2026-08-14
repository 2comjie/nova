package actorDef

import "time"

type Actor interface {
	OnStart(ctx ActorStartCtx) error
	OnUpdate(ctx ActorUpdateCtx) time.Duration
	OnStop(ctx ActorStopCtx) error
}

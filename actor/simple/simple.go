package actorSimple

import (
	"time"

	"github.com/2comjie/nova/actor/actorDef"
)

type SimpleActor struct {
	MUpdate func(ctx actorDef.ActorUpdateCtx) time.Duration
	MStart  func(ctx actorDef.ActorStartCtx) error
	MStop   func(ctx actorDef.ActorStopCtx)
}

func (s *SimpleActor) OnStart(ctx actorDef.ActorStartCtx) error {
	if s.MStart != nil {
		return s.MStart(ctx)
	}
	return nil
}

func (s *SimpleActor) OnUpdate(ctx actorDef.ActorUpdateCtx) time.Duration {
	if s.MUpdate != nil {
		return s.MUpdate(ctx)
	}
	return 0
}

func (s *SimpleActor) OnStop(ctx actorDef.ActorStopCtx) {
	if s.MStop != nil {
		s.MStop(ctx)
	}
}

package diff

import (
	"time"

	"github.com/2comjie/nova/actor/actorDef"
	"github.com/2comjie/nova/logx/logdef"
	"google.golang.org/protobuf/proto"
)

type LoadFunc[T actorDef.Actor, Pb proto.Message] func(diffActor *Actor[T, Pb], ctx actorDef.ActorStartCtx) (SnapState[Pb], uint64, error)
type PushFunc[T actorDef.Actor, Pb proto.Message] func(diffActor *Actor[T, Pb], data []byte)
type SaveFunc[T actorDef.Actor, Pb proto.Message] func(diffActor *Actor[T, Pb], data []byte)

type ActorConfig struct {
	ClientSaveDt time.Duration
	ServerSaveDt time.Duration
	DiffCount    uint64
}

type Actor[T actorDef.Actor, Pb proto.Message] struct {
	logdef.ILogger

	realActor T
	loader    LoadFunc[T, Pb]
	push      PushFunc[T, Pb]
	save      SaveFunc[T, Pb]
	config    ActorConfig

	self  actorDef.PID
	state SnapState[Pb]
	snap  *SnapManager[Pb]

	clientVersion uint64
	serverVersion uint64
	clientRemain  time.Duration
	serverRemain  time.Duration

	clientNeedSave bool
	serverNeedSave bool
}

func NewActor[T actorDef.Actor, Pb proto.Message](logger logdef.ILogger, realActor T, loader LoadFunc[T, Pb], push PushFunc[T, Pb], save SaveFunc[T, Pb], options ...ActorConfig) *Actor[T, Pb] {
	var config ActorConfig
	if len(options) == 1 {
		config = options[0]
	}
	if config.ClientSaveDt == 0 {
		config.ClientSaveDt = 100 * time.Millisecond
	}
	if config.ServerSaveDt == 0 {
		config.ServerSaveDt = 5 * time.Second
	}
	if config.DiffCount == 0 {
		config.DiffCount = 32
	}
	return &Actor[T, Pb]{
		ILogger:      logger,
		realActor:    realActor,
		loader:       loader,
		push:         push,
		save:         save,
		config:       config,
		clientRemain: config.ClientSaveDt,
		serverRemain: config.ServerSaveDt,
	}
}

func (a *Actor[T, Pb]) OnStart(ctx actorDef.ActorStartCtx) error {
	state, version, err := a.loader(a, ctx)
	if err != nil {
		return err
	}

	a.self = ctx.Self
	a.state = state
	a.snap = NewSnapManager(state, version, a.config.DiffCount)
	a.clientVersion = version
	a.serverVersion = version
	return a.realActor.OnStart(ctx)
}

func (a *Actor[T, Pb]) OnUpdate(ctx actorDef.ActorUpdateCtx) time.Duration {
	nextUpdate := a.realActor.OnUpdate(ctx)
	a.clientRemain -= ctx.Delta
	a.serverRemain -= ctx.Delta

	flushClient := a.clientNeedSave || a.clientRemain <= 0
	flushServer := a.serverNeedSave || a.serverRemain <= 0
	if flushClient || flushServer {
		a.snap.Commit()
	}

	if flushClient {
		a.clientRemain = a.config.ClientSaveDt
		a.pushClient()
	}
	if flushServer {
		a.serverRemain = a.config.ServerSaveDt
		a.saveServer()
	}

	if a.clientNeedSave && (nextUpdate <= 0 || a.clientRemain < nextUpdate) {
		nextUpdate = a.clientRemain
	}
	if a.serverNeedSave && (nextUpdate <= 0 || a.serverRemain < nextUpdate) {
		nextUpdate = a.serverRemain
	}
	return nextUpdate
}

func (a *Actor[T, Pb]) OnStop(ctx actorDef.ActorStopCtx) error {
	err := a.realActor.OnStop(ctx)
	if ctx.Reason == actorDef.StopReasonLeaseLost {
		return err
	}

	a.snap.Commit()
	a.saveServer()
	return err
}

func (a *Actor[T, Pb]) MarkNeedSave(reason ...string) {
	if len(reason) == 1 {
		a.Debugf("client save reason %s", reason[0])
	}
	a.clientNeedSave = true
}

func (a *Actor[T, Pb]) ForceSave(reason ...string) {
	if len(reason) == 1 {
		a.Infof("force save reason %s", reason[0])
	}
	a.clientNeedSave = true
	a.serverNeedSave = true
}

func (a *Actor[T, Pb]) ClientNeedSave() bool {
	return a.clientNeedSave
}

func (a *Actor[T, Pb]) ServerNeedSave() bool {
	return a.serverNeedSave
}

func (a *Actor[T, Pb]) RealActor() T {
	return a.realActor
}

func (a *Actor[T, Pb]) Self() actorDef.PID {
	return a.self
}

func (a *Actor[T, Pb]) Version() uint64 {
	return a.snap.Version()
}

func (a *Actor[T, Pb]) WriteSync(clientVersion uint64, buffer []byte) ([]byte, bool) {
	return a.snap.WriteSync(clientVersion, buffer)
}

func (a *Actor[T, Pb]) BuildSnapshot() (uint64, []byte) {
	return a.snap.BuildSnapshot()
}

func (a *Actor[T, Pb]) pushClient() {
	data, ok := a.snap.WriteSync(a.clientVersion, nil)
	if ok {
		a.push(a, data)
		a.clientVersion = a.snap.Version()
	}
	a.clientNeedSave = false
}

func (a *Actor[T, Pb]) saveServer() {
	data, ok := a.snap.WriteSync(a.serverVersion, nil)
	if ok {
		a.save(a, data)
		a.serverVersion = a.snap.Version()
	}
	a.serverNeedSave = false
}

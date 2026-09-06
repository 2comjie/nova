package actor

import (
	"context"
	"sync"

	"github.com/2comjie/nova/actor/actorDef"
	pbActor "github.com/2comjie/nova/internal/pb/transport/actor"
	"github.com/2comjie/nova/rpc"
)

type rpcProcessor func(ctx context.Context, key actorDef.Key, policy ActivationPolicy, message Message, needReply bool) ([]byte, bool, error)

type managerRegistration struct {
	stop   func()
	routes map[uint32]rpcProcessor
}

type System struct {
	pbActor.UnimplementedActorServer

	runCtx context.Context
	stop   context.CancelFunc
	done   chan struct{}

	registrations map[actorDef.Type]managerRegistration

	lifecycleMu sync.Mutex
	stopping    bool
	tasks       sync.WaitGroup
}

func NewSystem(registrar *rpc.Server) *System {
	runCtx, stop := context.WithCancel(context.Background())
	system := &System{
		runCtx:        runCtx,
		stop:          stop,
		done:          make(chan struct{}),
		registrations: make(map[actorDef.Type]managerRegistration),
	}
	pbActor.RegisterActorServer(registrar, system)
	return system
}

func (s *System) Start() error {
	return nil
}

func (s *System) RequestStop() {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.stopping {
		return
	}
	s.stopping = true
	for _, registration := range s.registrations {
		registration.stop()
	}
	s.stop()
	go func() {
		s.tasks.Wait()
		close(s.done)
	}()
}

func (s *System) Shutdown(ctx context.Context) error {
	s.RequestStop()
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *System) Ask(ctx context.Context, request *pbActor.Request) (*pbActor.Response, *rpc.Error) {
	return s.process(ctx, request, true)
}

func (s *System) Tell(ctx context.Context, request *pbActor.Request) (*pbActor.Response, *rpc.Error) {
	return s.process(ctx, request, false)
}

func (s *System) process(ctx context.Context, request *pbActor.Request, needReply bool) (*pbActor.Response, *rpc.Error) {
	if request.ActorKey == "" || request.Route == 0 {
		return nil, rpc.NewError(rpc.CodeInvalidArgument, "actor: RPC请求无效")
	}

	registration, exists := s.registrations[actorDef.Type(request.ActorType)]
	processor := registration.routes[request.Route]
	if !exists || processor == nil {
		return nil, rpc.NewError(rpc.CodeNotFound, "actor: RPC route不存在")
	}

	body, handled, err := processor(ctx, actorDef.Key(request.ActorKey), ActivationPolicy(request.Activation), Message{Route: request.Route, Body: request.Body}, needReply)
	if err != nil {
		return nil, rpc.FromError(err)
	}
	return &pbActor.Response{Handled: handled, Body: body}, nil
}

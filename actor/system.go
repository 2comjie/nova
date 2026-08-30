package actor

import (
	"context"
	"sync"

	"github.com/2comjie/nova/actor/actorDef"
	pbActor "github.com/2comjie/nova/internal/pb/transport/actor"
	"github.com/2comjie/nova/rpc/rpcerr"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

type rpcProcessor func(ctx context.Context, key actorDef.Key, policy ActivationPolicy, message Message, needReply bool) ([]byte, bool, error)

type managerController interface {
	requestStopAll()
}

type managerRegistration struct {
	manager managerController
	routes  map[uint32]rpcProcessor
}

type System struct {
	pbActor.UnimplementedActorServer

	runCtx context.Context
	stop   context.CancelFunc
	done   chan struct{}

	mu            sync.RWMutex
	registrations map[actorDef.Type]managerRegistration

	lifecycleMu sync.Mutex
	stopping    bool
	tasks       sync.WaitGroup
	stopOnce    sync.Once
}

func NewSystem(registrar grpc.ServiceRegistrar) *System {
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

func (s *System) Shutdown(ctx context.Context) error {
	s.stopOnce.Do(func() {
		s.lifecycleMu.Lock()
		s.stopping = true
		s.lifecycleMu.Unlock()

		s.mu.RLock()
		managers := make([]managerController, 0, len(s.registrations))
		for _, registration := range s.registrations {
			managers = append(managers, registration.manager)
		}
		s.mu.RUnlock()
		for _, manager := range managers {
			manager.requestStopAll()
		}
		s.stop()

		go func() {
			s.tasks.Wait()
			close(s.done)
		}()
	})

	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *System) Ask(ctx context.Context, request *pbActor.Request) (*pbActor.Response, rpcerr.Err) {
	return s.process(ctx, request, true)
}

func (s *System) Tell(ctx context.Context, request *pbActor.Request) (*pbActor.Response, rpcerr.Err) {
	return s.process(ctx, request, false)
}

func (s *System) process(ctx context.Context, request *pbActor.Request, needReply bool) (*pbActor.Response, rpcerr.Err) {
	if request == nil || request.ActorKey == "" || request.Route == 0 {
		return nil, rpcerr.NewGRPC(codes.InvalidArgument, "actor: RPC请求无效")
	}

	s.mu.RLock()
	registration, exists := s.registrations[actorDef.Type(request.ActorType)]
	processor := registration.routes[request.Route]
	s.mu.RUnlock()
	if !exists || processor == nil {
		return nil, rpcerr.NewGRPC(codes.NotFound, "actor: RPC route不存在")
	}

	body, handled, err := processor(ctx, actorDef.Key(request.ActorKey), ActivationPolicy(request.Activation), Message{Route: request.Route, Body: request.Body}, needReply)
	if err != nil {
		return nil, rpcerr.Wrap(err)
	}
	return &pbActor.Response{Handled: handled, Body: body}, nil
}

func (s *System) registerManager(actorType actorDef.Type, manager managerController) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.registrations[actorType]; exists {
		panic("actor: ActorType已经注册")
	}
	s.registrations[actorType] = managerRegistration{manager: manager, routes: make(map[uint32]rpcProcessor)}
}

func (s *System) registerRoute(actorType actorDef.Type, route uint32, processor rpcProcessor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	registration, exists := s.registrations[actorType]
	if !exists {
		panic("actor: Manager尚未注册")
	}
	if registration.routes[route] != nil {
		panic("actor: RPC route已经注册")
	}
	registration.routes[route] = processor
}

func (s *System) beginTask() bool {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.stopping {
		return false
	}
	s.tasks.Add(1)
	return true
}

func (s *System) endTask() {
	s.tasks.Done()
}

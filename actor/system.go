package actor

import (
	"context"
	"errors"
	"sync"

	"github.com/2comjie/wali/actor/actorDef"
	pbActor "github.com/2comjie/wali/internal/pb/transport/actor"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type rpcProcessor func(ctx context.Context, key actorDef.Key, policy ActivationPolicy, message Message, needReply bool) ([]byte, bool, error)

type System struct {
	pbActor.UnimplementedActorServer

	runCtx context.Context
	stop   context.CancelFunc
	done   chan struct{}

	mu         sync.RWMutex
	routes     map[actorDef.Type]map[uint32]rpcProcessor
	actorTypes map[actorDef.Type]struct{}

	lifecycleMu sync.Mutex
	stopping    bool
	tasks       sync.WaitGroup
	stopOnce    sync.Once

	errMu sync.Mutex
	err   error
}

func NewSystem(registrar grpc.ServiceRegistrar) *System {
	runCtx, stop := context.WithCancel(context.Background())
	system := &System{
		runCtx:     runCtx,
		stop:       stop,
		done:       make(chan struct{}),
		routes:     make(map[actorDef.Type]map[uint32]rpcProcessor),
		actorTypes: make(map[actorDef.Type]struct{}),
	}
	pbActor.RegisterActorServer(registrar, system)
	return system
}

func (s *System) Name() string {
	return "actor"
}

func (s *System) Start() error {
	return nil
}

func (s *System) Shutdown(ctx context.Context) error {
	s.stopOnce.Do(func() {
		s.lifecycleMu.Lock()
		s.stopping = true
		s.stop()
		s.lifecycleMu.Unlock()

		go func() {
			s.tasks.Wait()
			close(s.done)
		}()
	})

	select {
	case <-s.done:
		return s.Err()
	case <-ctx.Done():
		return errors.Join(ctx.Err(), s.Err())
	}
}

func (s *System) Ask(ctx context.Context, request *pbActor.Request) (*pbActor.Response, error) {
	return s.process(ctx, request, true)
}

func (s *System) Tell(ctx context.Context, request *pbActor.Request) (*pbActor.Response, error) {
	return s.process(ctx, request, false)
}

func (s *System) process(ctx context.Context, request *pbActor.Request, needReply bool) (*pbActor.Response, error) {
	if request == nil || request.ActorKey == "" || request.Route == 0 {
		return nil, status.Error(codes.InvalidArgument, "actor: RPC请求无效")
	}

	s.mu.RLock()
	processor := s.routes[actorDef.Type(request.ActorType)][request.Route]
	s.mu.RUnlock()
	if processor == nil {
		return nil, status.Error(codes.NotFound, "actor: RPC route不存在")
	}

	body, handled, err := processor(ctx, actorDef.Key(request.ActorKey), ActivationPolicy(request.Activation), Message{Route: request.Route, Body: request.Body}, needReply)
	if err != nil {
		return nil, err
	}
	return &pbActor.Response{Handled: handled, Body: body}, nil
}

func (s *System) registerActorType(actorType actorDef.Type) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.actorTypes[actorType]; exists {
		panic("actor: ActorType已经注册")
	}
	s.actorTypes[actorType] = struct{}{}
}

func (s *System) registerRoute(actorType actorDef.Type, route uint32, processor rpcProcessor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	routes := s.routes[actorType]
	if routes == nil {
		routes = make(map[uint32]rpcProcessor)
		s.routes[actorType] = routes
	}
	if routes[route] != nil {
		panic("actor: RPC route已经注册")
	}
	routes[route] = processor
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

func (s *System) isStopping() bool {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.stopping
}

func (s *System) addErr(err error) {
	if err == nil {
		return
	}
	s.errMu.Lock()
	s.err = errors.Join(s.err, err)
	s.errMu.Unlock()
}

func (s *System) Err() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.err
}

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

type Server struct {
	pbActor.UnimplementedActorServer

	mu     sync.RWMutex
	routes map[actorDef.Type]map[uint32]rpcProcessor
	stops  map[actorDef.Type]func() error
}

func NewServer(registrar grpc.ServiceRegistrar) *Server {
	server := &Server{
		routes: make(map[actorDef.Type]map[uint32]rpcProcessor),
		stops:  make(map[actorDef.Type]func() error),
	}
	pbActor.RegisterActorServer(registrar, server)
	return server
}

func (s *Server) Name() string {
	return "actor"
}

func (s *Server) Start() error {
	return nil
}

func (s *Server) Shutdown(context.Context) error {
	s.mu.RLock()
	stops := make([]func() error, 0, len(s.stops))
	for _, stop := range s.stops {
		stops = append(stops, stop)
	}
	s.mu.RUnlock()

	var stopErr error
	for _, stop := range stops {
		stopErr = errors.Join(stopErr, stop())
	}
	return stopErr
}

func (s *Server) Ask(ctx context.Context, request *pbActor.Request) (*pbActor.Response, error) {
	return s.process(ctx, request, true)
}

func (s *Server) Tell(ctx context.Context, request *pbActor.Request) (*pbActor.Response, error) {
	return s.process(ctx, request, false)
}

func (s *Server) process(ctx context.Context, request *pbActor.Request, needReply bool) (*pbActor.Response, error) {
	if request == nil || request.ActorKey == "" || request.Route == 0 {
		return nil, status.Error(codes.InvalidArgument, "actor: RPC请求无效")
	}

	s.mu.RLock()
	routes := s.routes[actorDef.Type(request.ActorType)]
	processor := routes[request.Route]
	s.mu.RUnlock()
	if processor == nil {
		return nil, status.Error(codes.NotFound, "actor: RPC route不存在")
	}

	body, handled, err := processor(
		ctx,
		actorDef.Key(request.ActorKey),
		ActivationPolicy(request.Activation),
		Message{Route: request.Route, Body: request.Body},
		needReply,
	)
	if err != nil {
		var redirect interface{ RedirectInstanceId() string }
		if errors.As(err, &redirect) {
			return &pbActor.Response{RedirectInstanceId: redirect.RedirectInstanceId()}, nil
		}
		return errorResponse(err), nil
	}
	return &pbActor.Response{Handled: handled, Body: body}, nil
}

func errorResponse(err error) *pbActor.Response {
	var actorError interface{ ActorErrorCode() uint32 }
	if errors.As(err, &actorError) {
		return &pbActor.Response{ErrorCode: actorError.ActorErrorCode(), ErrorMessage: err.Error()}
	}

	code := ErrorCodeExecutionFailed
	switch {
	case errors.Is(err, ErrActorNotActive):
		code = ErrorCodeActorNotActive
	case errors.Is(err, ErrInvalidActivationPolicy):
		code = ErrorCodeInvalidActivationPolicy
	case errors.Is(err, ErrSystemStopped):
		code = ErrorCodeSystemStopped
	}
	return &pbActor.Response{ErrorCode: code, ErrorMessage: err.Error()}
}

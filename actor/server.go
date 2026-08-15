package actor

import (
	"bytes"
	"context"
	"errors"
	"sync"

	"github.com/2comjie/wali/actor/actorDef"
	"github.com/2comjie/wali/app/node"
	"github.com/2comjie/wali/core/help"
	"github.com/2comjie/wali/logx"
)

var (
	ErrMessageRouteRegistered  = errors.New("actor message route registered")
	ErrMessageRouteNotFound    = errors.New("actor message route not found")
	ErrActorNotActive          = errors.New("actor not active")
	ErrInvalidActivationPolicy = errors.New("invalid actor activation policy")
	ErrMessageHandlerPanic     = errors.New("actor message handler panic")
)

type Handler[T actorDef.Actor] func(actorValue T, ctx *node.Context) error

type messageProcessor func(ctx *node.Context, key actorDef.Key, policy ActivationPolicy) (bool, error)

type Server struct {
	mu         sync.RWMutex
	askRoutes  map[uint32]messageProcessor
	tellRoutes map[uint32]messageProcessor
}

func NewServer() *Server {
	return &Server{askRoutes: make(map[uint32]messageProcessor), tellRoutes: make(map[uint32]messageProcessor)}
}

func resolveActor[T actorDef.Actor](ctx context.Context, system *System[T], key actorDef.Key, policy ActivationPolicy) (*Runner[T], bool, error) {
	var runner *Runner[T]
	var err error

	switch policy {
	case ActivationLoad:
		runner, err = system.GetOrLoadActor(ctx, key)
	case ActivationIgnore:
		var exists bool
		runner, exists = system.TryGetActor(key)
		if !exists {
			return nil, false, nil
		}
	case ActivationRequire:
		var exists bool
		runner, exists = system.TryGetActor(key)
		if !exists {
			return nil, false, ErrActorNotActive
		}
	default:
		return nil, false, ErrInvalidActivationPolicy
	}
	if err != nil {
		return nil, false, err
	}
	return runner, true, nil
}

func RegAsk[T actorDef.Actor](server *Server, system *System[T], route uint32, handler Handler[T]) error {
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.askRoutes[route] != nil || server.tellRoutes[route] != nil {
		return ErrMessageRouteRegistered
	}

	server.askRoutes[route] = func(ctx *node.Context, key actorDef.Key, policy ActivationPolicy) (bool, error) {
		runner, handled, err := resolveActor(ctx, system, key, policy)
		if err != nil || !handled {
			return handled, err
		}

		handleErr := ErrMessageHandlerPanic
		err = runner.WaitResultOnMainLoop(ctx, func(actorValue T) {
			help.SafeRun(func() {
				handleErr = handler(actorValue, ctx)
			})
		})
		if err != nil {
			return false, err
		}
		return true, handleErr
	}
	return nil
}

func RegTell[T actorDef.Actor](server *Server, system *System[T], route uint32, handler Handler[T]) error {
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.askRoutes[route] != nil || server.tellRoutes[route] != nil {
		return ErrMessageRouteRegistered
	}

	server.tellRoutes[route] = func(ctx *node.Context, key actorDef.Key, policy ActivationPolicy) (bool, error) {
		runner, handled, err := resolveActor(ctx, system, key, policy)
		if err != nil || !handled {
			return handled, err
		}

		tellCtx := *ctx
		request := *ctx.Request
		request.Body = bytes.Clone(request.Body)
		tellCtx.Context = runner.runCtx
		tellCtx.Request = &request

		err = runner.RunOnMainLoop(func(actorValue T) {
			handleErr := ErrMessageHandlerPanic
			help.SafeRun(func() {
				handleErr = handler(actorValue, &tellCtx)
			})
			if handleErr != nil {
				logx.Errorf("actor: Tell处理失败 route=%d key=%s err=%v", request.Route, key, handleErr)
			}
		})
		return err == nil, err
	}
	return nil
}

func (s *Server) HasRoute(route uint32) bool {
	s.mu.RLock()
	askProcessor := s.askRoutes[route]
	tellProcessor := s.tellRoutes[route]
	s.mu.RUnlock()
	return askProcessor != nil || tellProcessor != nil
}

func (s *Server) Handle(ctx *node.Context, key actorDef.Key, policy ActivationPolicy) error {
	s.mu.RLock()
	processor := s.tellRoutes[ctx.Request.Route]
	if ctx.NeedReply() {
		processor = s.askRoutes[ctx.Request.Route]
	}
	s.mu.RUnlock()
	if processor == nil {
		return ErrMessageRouteNotFound
	}

	_, err := processor(ctx, key, policy)
	return err
}

func (s *Server) Ask(ctx context.Context, key actorDef.Key, policy ActivationPolicy, message Message) ([]byte, bool, error) {
	s.mu.RLock()
	processor := s.askRoutes[message.Route]
	s.mu.RUnlock()
	if processor == nil {
		return nil, false, ErrMessageRouteNotFound
	}

	nodeCtx := &node.Context{
		Context: ctx,
		Request: &node.Request{Route: message.Route, Body: message.Body, NeedReply: true},
	}
	handled, err := processor(nodeCtx, key, policy)
	return nodeCtx.ResponseBody(), handled, err
}

func (s *Server) Tell(ctx context.Context, key actorDef.Key, policy ActivationPolicy, message Message) error {
	s.mu.RLock()
	processor := s.tellRoutes[message.Route]
	s.mu.RUnlock()
	if processor == nil {
		return ErrMessageRouteNotFound
	}

	nodeCtx := &node.Context{Context: ctx, Request: &node.Request{Route: message.Route, Body: message.Body}}
	_, err := processor(nodeCtx, key, policy)
	return err
}

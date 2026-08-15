package actor

import (
	"bytes"
	"context"
	"errors"
	"sync"

	"github.com/2comjie/wali/actor/actorDef"
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

type AskHandler[T actorDef.Actor] func(ctx context.Context, actorValue T, message Message) ([]byte, error)

type TellHandler[T actorDef.Actor] func(ctx context.Context, actorValue T, message Message) error

type askProcessor func(ctx context.Context, key actorDef.Key, policy ActivationPolicy, message Message) ([]byte, bool, error)

type tellProcessor func(ctx context.Context, key actorDef.Key, policy ActivationPolicy, message Message) error

type Server struct {
	mu         sync.RWMutex
	askRoutes  map[uint32]askProcessor
	tellRoutes map[uint32]tellProcessor
}

func NewServer() *Server {
	return &Server{askRoutes: make(map[uint32]askProcessor), tellRoutes: make(map[uint32]tellProcessor)}
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

func RegAsk[T actorDef.Actor](server *Server, system *System[T], route uint32, handler AskHandler[T]) error {
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.askRoutes[route] != nil || server.tellRoutes[route] != nil {
		return ErrMessageRouteRegistered
	}

	server.askRoutes[route] = func(ctx context.Context, key actorDef.Key, policy ActivationPolicy, message Message) ([]byte, bool, error) {
		runner, handled, err := resolveActor(ctx, system, key, policy)
		if err != nil || !handled {
			return nil, handled, err
		}

		result := make(chan struct {
			body []byte
			err  error
		}, 1)
		err = runner.WaitResultOnMainLoop(ctx, func(actorValue T) {
			body := []byte(nil)
			handleErr := ErrMessageHandlerPanic
			help.SafeRun(func() {
				body, handleErr = handler(ctx, actorValue, message)
			})
			result <- struct {
				body []byte
				err  error
			}{body: body, err: handleErr}
		})
		if err != nil {
			return nil, false, err
		}
		handleResult := <-result
		return handleResult.body, true, handleResult.err
	}
	return nil
}

func RegTell[T actorDef.Actor](server *Server, system *System[T], route uint32, handler TellHandler[T]) error {
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.askRoutes[route] != nil || server.tellRoutes[route] != nil {
		return ErrMessageRouteRegistered
	}

	server.tellRoutes[route] = func(ctx context.Context, key actorDef.Key, policy ActivationPolicy, message Message) error {
		runner, handled, err := resolveActor(ctx, system, key, policy)
		if err != nil || !handled {
			return err
		}

		message.Body = bytes.Clone(message.Body)
		return runner.RunOnMainLoop(func(actorValue T) {
			handleErr := ErrMessageHandlerPanic
			help.SafeRun(func() {
				handleErr = handler(runner.runCtx, actorValue, message)
			})
			if handleErr != nil {
				logx.Errorf("actor: Tell处理失败 route=%d key=%s err=%v", message.Route, key, handleErr)
			}
		})
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

func (s *Server) Ask(ctx context.Context, key actorDef.Key, policy ActivationPolicy, message Message) ([]byte, bool, error) {
	s.mu.RLock()
	processor := s.askRoutes[message.Route]
	s.mu.RUnlock()
	if processor == nil {
		return nil, false, ErrMessageRouteNotFound
	}
	return processor(ctx, key, policy, message)
}

func (s *Server) Tell(ctx context.Context, key actorDef.Key, policy ActivationPolicy, message Message) error {
	s.mu.RLock()
	processor := s.tellRoutes[message.Route]
	s.mu.RUnlock()
	if processor == nil {
		return ErrMessageRouteNotFound
	}
	return processor(ctx, key, policy, message)
}

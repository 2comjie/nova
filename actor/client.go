package actor

import (
	"context"

	"github.com/2comjie/wali/actor/actorDef"
	pbActor "github.com/2comjie/wali/internal/pb/transport/actor"
	rpcClient "github.com/2comjie/wali/rpc/client"
	"github.com/2comjie/wali/rpc/lx"
)

type Client struct {
	rpc pbActor.ActorClient
}

type Ref struct {
	client  *Client
	service string
	pid     actorDef.PID
	policy  ActivationPolicy
}

func NewClient(client *rpcClient.Client) *Client {
	return &Client{rpc: pbActor.NewActorClient(client)}
}

func (c *Client) Ref(service string, pid actorDef.PID, policy ActivationPolicy) Ref {
	return Ref{client: c, service: service, pid: pid, policy: policy}
}

func (r Ref) Ask(ctx context.Context, message Message) ([]byte, bool, error) {
	request := &pbActor.Request{
		ActorType:  int32(r.pid.Type),
		ActorKey:   string(r.pid.Key),
		Activation: uint32(r.policy),
		Route:      message.Route,
		Body:       message.Body,
	}
	response, err := r.client.rpc.Ask(lx.WithActor(ctx, r.service, string(r.pid.Key)), request)
	if err != nil {
		return nil, false, err
	}
	if response.RedirectInstanceId != "" {
		response, err = r.client.rpc.Ask(lx.WithNode(ctx, response.RedirectInstanceId), request)
		if err != nil {
			return nil, false, err
		}
	}
	if response.RedirectInstanceId != "" {
		return nil, false, ActorRedirectError(response.RedirectInstanceId)
	}
	if response.ErrorCode != 0 {
		return nil, false, &CallError{Code: response.ErrorCode, Message: response.ErrorMessage}
	}
	return response.Body, response.Handled, nil
}

func (r Ref) Tell(ctx context.Context, message Message) error {
	request := &pbActor.Request{
		ActorType:  int32(r.pid.Type),
		ActorKey:   string(r.pid.Key),
		Activation: uint32(r.policy),
		Route:      message.Route,
		Body:       message.Body,
	}
	response, err := r.client.rpc.Tell(lx.WithActor(ctx, r.service, string(r.pid.Key)), request)
	if err != nil {
		return err
	}
	if response.RedirectInstanceId != "" {
		response, err = r.client.rpc.Tell(lx.WithNode(ctx, response.RedirectInstanceId), request)
		if err != nil {
			return err
		}
	}
	if response.RedirectInstanceId != "" {
		return ActorRedirectError(response.RedirectInstanceId)
	}
	if response.ErrorCode != 0 {
		return &CallError{Code: response.ErrorCode, Message: response.ErrorMessage}
	}
	return nil
}

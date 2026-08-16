package actor

import (
	"context"

	"github.com/2comjie/nova/actor/actorDef"
	pbActor "github.com/2comjie/nova/internal/pb/transport/actor"
	rpcClient "github.com/2comjie/nova/rpc/client"
	"github.com/2comjie/nova/rpc/lx"
	"github.com/2comjie/nova/rpc/rpcerr"
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

func (r Ref) Ask(ctx context.Context, message Message) ([]byte, bool, rpcerr.Err) {
	request := &pbActor.Request{
		ActorType:  int32(r.pid.Type),
		ActorKey:   string(r.pid.Key),
		Activation: uint32(r.policy),
		Route:      message.Route,
		Body:       message.Body,
	}
	response, err := r.client.rpc.Ask(lx.WithActor(ctx, r.service, string(r.pid.Key)), request)
	if err != nil && err.Code() == ErrorCodeActorRedirect {
		response, err = r.client.rpc.Ask(lx.WithNode(ctx, string(err.Detail())), request)
	}
	if err != nil {
		return nil, false, err
	}
	return response.Body, response.Handled, nil
}

func (r Ref) Tell(ctx context.Context, message Message) rpcerr.Err {
	request := &pbActor.Request{
		ActorType:  int32(r.pid.Type),
		ActorKey:   string(r.pid.Key),
		Activation: uint32(r.policy),
		Route:      message.Route,
		Body:       message.Body,
	}
	_, err := r.client.rpc.Tell(lx.WithActor(ctx, r.service, string(r.pid.Key)), request)
	if err != nil && err.Code() == ErrorCodeActorRedirect {
		_, err = r.client.rpc.Tell(lx.WithNode(ctx, string(err.Detail())), request)
	}
	return err
}

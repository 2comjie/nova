package node

import (
	"context"
	"errors"

	pbNode "github.com/2comjie/nova/internal/pb/transport/node"
	"github.com/2comjie/nova/locator"
	"github.com/2comjie/nova/rpc"
)

func (n *Node) Call(ctx context.Context, request *pbNode.Request) (*pbNode.Response, *rpc.Error) {
	nodeContext, err := n.handle(ctx, request, true)
	if err != nil {
		return nil, rpc.FromError(err)
	}
	return &pbNode.Response{
		Replied:         nodeContext.replied,
		Body:            nodeContext.responseBody,
		NodeServiceName: n.instance.ServiceName,
		NodeInstanceId:  n.instance.Id,
	}, nil
}

func (n *Node) Tell(ctx context.Context, request *pbNode.Request) (*pbNode.Response, *rpc.Error) {
	if _, err := n.handle(ctx, request, false); err != nil {
		return nil, rpc.FromError(err)
	}
	return &pbNode.Response{NodeServiceName: n.instance.ServiceName, NodeInstanceId: n.instance.Id}, nil
}

func (n *Node) handle(ctx context.Context, request *pbNode.Request, needReply bool) (*Context, error) {
	if request.Uid == 0 || request.Route == 0 || request.GateServiceName != locator.GateName || request.GateInstanceId == "" {
		return nil, rpc.NewError(rpc.CodeInvalidArgument, "node: 请求参数无效")
	}

	nodeContext := &Context{
		Context: ctx,
		App:     n,
		Request: &Request{
			Route:           request.Route,
			Uid:             request.Uid,
			Body:            request.Body,
			GateServiceName: request.GateServiceName,
			GateInstanceId:  request.GateInstanceId,
			ActorKey:        request.ActorKey,
			NeedReply:       needReply,
		},
	}

	if err := n.router.Dispatch(nodeContext); err != nil {
		if errors.Is(err, ErrRouteNotFound) {
			return nil, rpc.NewError(rpc.CodeNotFound, "node: route不存在")
		}
		return nil, err
	}
	return nodeContext, nil
}

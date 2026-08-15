package node

import (
	"context"
	"errors"

	"github.com/2comjie/wali/core/help"
	pbNode "github.com/2comjie/wali/internal/pb/transport/node"
	"github.com/2comjie/wali/locator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (n *Node) Call(ctx context.Context, request *pbNode.Request) (*pbNode.Response, error) {
	nodeContext, err := n.handle(ctx, request, true)
	if err != nil {
		if redirectInstanceId := actorRedirectInstanceId(err); redirectInstanceId != "" {
			return &pbNode.Response{
				NodeServiceName:    n.instance.ServiceName,
				NodeInstanceId:     n.instance.ID,
				RedirectInstanceId: redirectInstanceId,
			}, nil
		}
		return nil, err
	}
	return &pbNode.Response{
		Replied:         nodeContext.replied,
		Body:            nodeContext.responseBody,
		NodeServiceName: n.instance.ServiceName,
		NodeInstanceId:  n.instance.ID,
	}, nil
}

func (n *Node) Tell(ctx context.Context, request *pbNode.Request) (*pbNode.Response, error) {
	if _, err := n.handle(ctx, request, false); err != nil {
		if redirectInstanceId := actorRedirectInstanceId(err); redirectInstanceId != "" {
			return &pbNode.Response{
				NodeServiceName:    n.instance.ServiceName,
				NodeInstanceId:     n.instance.ID,
				RedirectInstanceId: redirectInstanceId,
			}, nil
		}
		return nil, err
	}
	return &pbNode.Response{NodeServiceName: n.instance.ServiceName, NodeInstanceId: n.instance.ID}, nil
}

func (n *Node) handle(ctx context.Context, request *pbNode.Request, needReply bool) (*Context, error) {
	if request == nil || request.Uid == "" || request.Route == 0 || request.GateServiceName != locator.GateName || request.GateInstanceId == "" {
		return nil, status.Error(codes.InvalidArgument, "node: 请求参数无效")
	}

	nodeContext := &Context{
		Context: ctx,
		App:     n,
		Request: &Request{
			Route:           request.Route,
			UID:             request.Uid,
			Body:            request.Body,
			GateServiceName: request.GateServiceName,
			GateInstanceID:  request.GateInstanceId,
			ActorKey:        request.ActorKey,
			NeedReply:       needReply,
		},
	}

	var dispatchErr error
	completed := false
	help.SafeRun(func() {
		dispatchErr = n.router.Dispatch(nodeContext)
		completed = true
	})
	if !completed {
		return nil, status.Error(codes.Internal, "node: Handler执行失败")
	}
	if dispatchErr != nil {
		if actorRedirectInstanceId(dispatchErr) != "" {
			return nil, dispatchErr
		}
		if errors.Is(dispatchErr, ErrRouteNotFound) {
			return nil, status.Error(codes.NotFound, "node: route不存在")
		}
		return nil, status.Error(codes.Internal, "node: 请求处理失败")
	}
	return nodeContext, nil
}

func actorRedirectInstanceId(err error) string {
	var redirect interface{ RedirectInstanceId() string }
	if errors.As(err, &redirect) {
		return redirect.RedirectInstanceId()
	}
	return ""
}

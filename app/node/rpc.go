package node

import (
	"context"
	"errors"

	"github.com/2comjie/wali/core/help"
	pbNode "github.com/2comjie/wali/internal/pb/transport/node"
	"github.com/2comjie/wali/locator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Call 执行业务请求，业务层调用Context.Reply后才响应客户端。
func (n *Node) Call(ctx context.Context, request *pbNode.Request) (*pbNode.Response, error) {
	nodeContext, err := n.handle(ctx, request, true)
	if err != nil {
		return nil, err
	}
	return &pbNode.Response{
		Replied:         nodeContext.replied,
		Body:            nodeContext.responseBody,
		NodeServiceName: n.instance.ServiceName,
		NodeInstanceId:  n.instance.ID,
	}, nil
}

// Tell 执行业务请求，但不允许响应客户端。
func (n *Node) Tell(ctx context.Context, request *pbNode.Request) (*emptypb.Empty, error) {
	if _, err := n.handle(ctx, request, false); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (n *Node) handle(ctx context.Context, request *pbNode.Request, needReply bool) (*Context, error) {
	if request == nil || request.Uid == "" || request.Route == 0 || request.GateServiceName != locator.GateName || request.GateInstanceId == "" {
		return nil, status.Error(codes.InvalidArgument, "node: 请求参数无效")
	}

	nodeContext := &Context{
		Context: ctx,
		App:     n.proxy,
		Request: &Request{
			Route:           request.Route,
			UID:             request.Uid,
			Body:            request.Body,
			GateServiceName: request.GateServiceName,
			GateInstanceID:  request.GateInstanceId,
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
		if errors.Is(dispatchErr, ErrRouteNotFound) {
			return nil, status.Error(codes.NotFound, "node: route不存在")
		}
		return nil, status.Error(codes.Internal, "node: 请求处理失败")
	}
	return nodeContext, nil
}

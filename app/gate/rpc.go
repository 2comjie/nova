package gate

import (
	"context"
	"errors"

	pbGate "github.com/2comjie/wali/internal/pb/transport/gate"
	"github.com/2comjie/wali/network"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (g *Gate) Push(ctx context.Context, request *pbGate.PushRequest) (*emptypb.Empty, error) {
	if request == nil || request.Uid == "" || request.Route == 0 ||
		request.NodeServiceName == "" || request.NodeInstanceId == "" {
		return nil, status.Error(codes.InvalidArgument, "gate: Push参数无效")
	}
	if err := g.server.PushUID(ctx, request.Uid, request.Route, request.Body); err != nil {
		if errors.Is(err, network.ErrNotBound) {
			return nil, status.Error(codes.NotFound, "gate: UID不在线")
		}
		return nil, status.Error(codes.Unavailable, "gate: Push失败")
	}
	return &emptypb.Empty{}, nil
}

func (g *Gate) Kick(ctx context.Context, request *pbGate.KickRequest) (*emptypb.Empty, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request == nil || request.Uid == "" ||
		request.NodeServiceName == "" || request.NodeInstanceId == "" {
		return nil, status.Error(codes.InvalidArgument, "gate: Kick参数无效")
	}
	if request.SessionId != 0 {
		g.server.KickUIDSession(request.Uid, request.SessionId)
	} else {
		g.server.KickUID(request.Uid)
	}
	return &emptypb.Empty{}, nil
}

func (g *Gate) Broadcast(ctx context.Context, request *pbGate.BroadcastRequest) (*pbGate.BroadcastResponse, error) {
	if request == nil || request.Route == 0 ||
		request.NodeServiceName == "" || request.NodeInstanceId == "" {
		return nil, status.Error(codes.InvalidArgument, "gate: Broadcast参数无效")
	}
	count, err := g.server.Broadcast(ctx, request.Route, request.Body)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "gate: Broadcast失败")
	}
	return &pbGate.BroadcastResponse{
		Count: count,
	}, nil
}

func (g *Gate) MultiPush(ctx context.Context, request *pbGate.MultiPushRequest) (*pbGate.MultiPushResponse, error) {
	if request == nil || request.NodeServiceName == "" || request.NodeInstanceId == "" {
		return nil, status.Error(codes.InvalidArgument, "gate: MultiPush参数无效")
	}
	count, err := g.server.MultiPush(ctx, request.UidList, request.Route, request.Body)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "gate: MultiPush失败")
	}
	return &pbGate.MultiPushResponse{
		Count: count,
	}, nil
}

func (g *Gate) MockCall(ctx context.Context, request *pbGate.MockCallRequest) (*pbGate.MockCallResponse, error) {
	if request == nil || request.Uid == "" || request.Route == 0 ||
		request.NodeServiceName == "" || request.NodeInstanceId == "" {
		return nil, status.Error(codes.InvalidArgument, "gate: MockCall参数无效")
	}
	gateCtx := &Context{
		Context:    ctx,
		App:        g,
		Uid:        request.Uid,
		Route:      request.Route,
		Body:       request.Body,
		BindingKey: request.Uid,
		needReply:  true,
		forward:    g.forward,
	}
	if err := g.dispatch(gateCtx); err != nil {
		return nil, err
	}
	return &pbGate.MockCallResponse{Replied: gateCtx.replied, Body: gateCtx.responseBody}, nil
}

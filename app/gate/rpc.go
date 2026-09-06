package gate

import (
	"context"
	"errors"
	"strconv"

	pbGate "github.com/2comjie/nova/internal/pb/transport/gate"
	"github.com/2comjie/nova/network"
	"github.com/2comjie/nova/rpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (g *Gate) Push(ctx context.Context, request *pbGate.PushRequest) (*emptypb.Empty, *rpc.Error) {
	if request.Uid == 0 || request.Route == 0 ||
		request.NodeServiceName == "" || request.NodeInstanceId == "" {
		return nil, rpc.NewError(rpc.CodeInvalidArgument, "gate: Push参数无效")
	}
	if err := g.server.PushUid(ctx, request.Uid, request.Route, request.Body); err != nil {
		if errors.Is(err, network.ErrNotBound) {
			return nil, rpc.NewError(rpc.CodeNotFound, "gate: UID不在线")
		}
		return nil, rpc.FromError(err)
	}
	return &emptypb.Empty{}, nil
}

func (g *Gate) Kick(ctx context.Context, request *pbGate.KickRequest) (*emptypb.Empty, *rpc.Error) {
	if request.Uid == 0 ||
		request.NodeServiceName == "" || request.NodeInstanceId == "" {
		return nil, rpc.NewError(rpc.CodeInvalidArgument, "gate: Kick参数无效")
	}
	if request.SessionId != 0 {
		g.server.KickUidSession(request.Uid, request.SessionId)
	} else {
		g.server.KickUid(request.Uid)
	}
	return &emptypb.Empty{}, nil
}

func (g *Gate) Broadcast(ctx context.Context, request *pbGate.BroadcastRequest) (*pbGate.BroadcastResponse, *rpc.Error) {
	if request.Route == 0 ||
		request.NodeServiceName == "" || request.NodeInstanceId == "" {
		return nil, rpc.NewError(rpc.CodeInvalidArgument, "gate: Broadcast参数无效")
	}
	count, err := g.server.Broadcast(ctx, request.Route, request.Body)
	if err != nil {
		return nil, rpc.FromError(err)
	}
	return &pbGate.BroadcastResponse{
		Count: count,
	}, nil
}

func (g *Gate) MultiPush(ctx context.Context, request *pbGate.MultiPushRequest) (*pbGate.MultiPushResponse, *rpc.Error) {
	if request.Route == 0 || request.NodeServiceName == "" || request.NodeInstanceId == "" {
		return nil, rpc.NewError(rpc.CodeInvalidArgument, "gate: MultiPush参数无效")
	}
	count, err := g.server.MultiPush(ctx, request.UidList, request.Route, request.Body)
	if err != nil {
		return nil, rpc.FromError(err)
	}
	return &pbGate.MultiPushResponse{
		Count: count,
	}, nil
}

func (g *Gate) MockCall(ctx context.Context, request *pbGate.MockCallRequest) (*pbGate.MockCallResponse, *rpc.Error) {
	if request.Uid == 0 || request.Route == 0 ||
		request.NodeServiceName == "" || request.NodeInstanceId == "" {
		return nil, rpc.NewError(rpc.CodeInvalidArgument, "gate: MockCall参数无效")
	}
	gateCtx := &Context{
		Context:    ctx,
		App:        g,
		Uid:        request.Uid,
		Route:      request.Route,
		Body:       request.Body,
		BindingKey: strconv.FormatUint(request.Uid, 10),
		needReply:  true,
		forward:    g.forward,
	}
	if err := g.router.Dispatch(gateCtx); err != nil {
		return nil, rpc.FromError(err)
	}
	return &pbGate.MockCallResponse{Replied: gateCtx.replied, Body: gateCtx.responseBody}, nil
}

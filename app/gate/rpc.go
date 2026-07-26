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

// Push 向UID当前绑定的客户端连接推送消息。
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

// Kick 关闭UID当前绑定的客户端连接，不在线时同样返回成功。
func (g *Gate) Kick(ctx context.Context, request *pbGate.KickRequest) (*emptypb.Empty, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if request == nil || request.Uid == "" ||
		request.NodeServiceName == "" || request.NodeInstanceId == "" {
		return nil, status.Error(codes.InvalidArgument, "gate: Kick参数无效")
	}
	g.server.KickUID(request.Uid)
	return &emptypb.Empty{}, nil
}

package gate

import (
	"context"

	clusterGate "github.com/2comjie/wali/cluster/gate"
	pbGate "github.com/2comjie/wali/internal/pb/transport/gate"
)

type Gate struct {
	pbGate.UnimplementedGateServer
	gate *clusterGate.Gate
}

func New(g *clusterGate.Gate) *Gate {
	return &Gate{gate: g}
}

func (g *Gate) BindUid(ctx context.Context, req *pbGate.BindUidRequest) (*pbGate.BindUidResponse, error) {
	if err := g.gate.BindUid(ctx, req.GetUid(), req.GetConnId()); err != nil {
		return nil, err
	}
	return &pbGate.BindUidResponse{}, nil
}

func (g *Gate) UnbindUid(ctx context.Context, req *pbGate.UnbindUidRequest) (*pbGate.UnbindUidResponse, error) {
	g.gate.UnbindUid(ctx, req.GetConnId())
	return &pbGate.UnbindUidResponse{}, nil
}

func (g *Gate) Push(ctx context.Context, req *pbGate.PushRequest) (*pbGate.PushMessageResponse, error) {
	if err := g.gate.Push(ctx, req.GetConnId(), req.GetUid(), req.GetMessage()); err != nil {
		return nil, err
	}
	return &pbGate.PushMessageResponse{}, nil
}

func (g *Gate) BroadCast(ctx context.Context, req *pbGate.BroadCastRequest) (*pbGate.BroadCastResponse, error) {
	count, total := g.gate.BroadCast(ctx, req.GetMessage())
	return &pbGate.BroadCastResponse{Count: count, Total: total}, nil
}

func (g *Gate) MultiCast(ctx context.Context, req *pbGate.MultiCastRequest) (*pbGate.MultiCastResponse, error) {
	count, total := g.gate.MultiCast(ctx, req.GetUids(), req.GetConnIds(), req.GetMessage())
	return &pbGate.MultiCastResponse{Count: count, Total: total}, nil
}

func (g *Gate) Kick(ctx context.Context, req *pbGate.KickRequest) (*pbGate.KickResponse, error) {
	if err := g.gate.Kick(ctx, req.GetConnId(), req.GetUid(), req.GetReason()); err != nil {
		return nil, err
	}
	return &pbGate.KickResponse{}, nil
}

func (g *Gate) Stat(ctx context.Context, req *pbGate.StatRequest) (*pbGate.StatResponse, error) {
	return &pbGate.StatResponse{Total: g.gate.Stat(ctx)}, nil
}

func (g *Gate) GetIP(ctx context.Context, req *pbGate.GetIPRequest) (*pbGate.GetIPResponse, error) {
	ip, err := g.gate.GetIP(ctx, req.GetConnId(), req.GetUid())
	if err != nil {
		return nil, err
	}
	return &pbGate.GetIPResponse{Ip: ip}, nil
}

func (g *Gate) IsOnline(ctx context.Context, req *pbGate.IsOnlineRequest) (*pbGate.IsOnlineResponse, error) {
	return &pbGate.IsOnlineResponse{Online: g.gate.IsOnline(ctx, req.GetConnId(), req.GetUid())}, nil
}

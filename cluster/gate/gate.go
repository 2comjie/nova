package gate

import (
	"context"

	networkServer "github.com/2comjie/wali/network/server"
)

type Gate struct {
	netServer networkServer.NetServer
}

func New(netServer networkServer.NetServer) *Gate {
	return &Gate{netServer: netServer}
}

func (g *Gate) BindUid(ctx context.Context, uid string, cid int64) error {
	return g.netServer.Bind(cid, uid)
}

func (g *Gate) UnbindUid(ctx context.Context, cid int64) {
	g.netServer.Unbind(cid)
}

func (g *Gate) Push(ctx context.Context, cid int64, uid string, message []byte) error {
	if uid != "" {
		return g.netServer.WriteToUid(uid, message)
	}
	return g.netServer.WriteToCid(cid, message)
}

func (g *Gate) BroadCast(ctx context.Context, message []byte) (count, total int64) {
	g.netServer.Range(func(conn networkServer.Conn) bool {
		total++
		if conn.IsOpen() {
			if err := conn.Write(message); err == nil {
				count++
			}
		}
		return true
	})
	return
}

func (g *Gate) MultiCast(ctx context.Context, uids []string, connIds []int64, message []byte) (count, total int64) {
	for _, uid := range uids {
		if uid == "" {
			continue
		}
		total++
		if err := g.netServer.WriteToUid(uid, message); err == nil {
			count++
		}
	}
	for _, cid := range connIds {
		if cid == 0 {
			continue
		}
		total++
		if err := g.netServer.WriteToCid(cid, message); err == nil {
			count++
		}
	}
	return
}

func (g *Gate) Kick(ctx context.Context, cid int64, uid string, reason string) error {
	if uid != "" {
		conn := g.netServer.ConnByUid(uid)
		if conn == nil {
			return networkServer.ErrConnNotFound
		}
		g.netServer.CloseConn(conn.ID(), reason)
		return nil
	}
	g.netServer.CloseConn(cid, reason)
	return nil
}

func (g *Gate) Stat(ctx context.Context) int64 {
	return int64(g.netServer.ConnCount())
}

func (g *Gate) GetIP(ctx context.Context, cid int64, uid string) (string, error) {
	if uid != "" {
		conn := g.netServer.ConnByUid(uid)
		if conn == nil {
			return "", networkServer.ErrConnNotFound
		}
		return conn.RemoteAddr().String(), nil
	}
	conn := g.netServer.ConnById(cid)
	if conn == nil {
		return "", networkServer.ErrConnNotFound
	}
	return conn.RemoteAddr().String(), nil
}

func (g *Gate) IsOnline(ctx context.Context, cid int64, uid string) bool {
	if uid != "" {
		conn := g.netServer.ConnByUid(uid)
		return conn != nil && conn.IsOpen()
	}
	conn := g.netServer.ConnById(cid)
	return conn != nil && conn.IsOpen()
}

package client

import (
	"sync"

	"github.com/2comjie/nova/rpc"
)

type ConnPool struct {
	mu      sync.Mutex
	connMap map[string]*rpc.Conn
	opts    []rpc.ConnOption
}

func NewConnPool(opts ...rpc.ConnOption) *ConnPool {
	return &ConnPool{
		connMap: make(map[string]*rpc.Conn),
		opts:    opts,
	}
}

func (p *ConnPool) Get(addr string) (*rpc.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.connMap == nil {
		return nil, ErrClosed
	}
	if conn := p.connMap[addr]; conn != nil {
		return conn, nil
	}

	conn := rpc.NewConn(addr, p.opts...)
	p.connMap[addr] = conn
	return conn, nil
}

func (p *ConnPool) Close() {
	p.mu.Lock()
	connections := p.connMap
	p.connMap = nil
	p.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
}

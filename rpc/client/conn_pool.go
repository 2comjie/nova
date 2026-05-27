package client

import (
	"sync"

	"google.golang.org/grpc"
)

type ConnPool struct {
	mu    sync.RWMutex
	conns map[string]*grpc.ClientConn
	opts  []grpc.DialOption
}

func NewConnPool(opts ...grpc.DialOption) *ConnPool {
	return &ConnPool{
		conns: make(map[string]*grpc.ClientConn),
		opts:  opts,
	}
}

func (p *ConnPool) Get(addr string) (*grpc.ClientConn, error) {
	if addr == "" {
		return nil, ErrInvalidTarget
	}

	p.mu.RLock()
	conn := p.conns[addr]
	p.mu.RUnlock()
	if conn != nil {
		return conn, nil
	}

	conn, err := grpc.NewClient(addr, p.opts...)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	if old := p.conns[addr]; old != nil {
		p.mu.Unlock()
		_ = conn.Close()
		return old, nil
	}
	p.conns[addr] = conn
	p.mu.Unlock()

	return conn, nil
}

func (p *ConnPool) Prune(activeAddrs map[string]bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for addr, conn := range p.conns {
		if !activeAddrs[addr] {
			_ = conn.Close()
			delete(p.conns, addr)
		}
	}
}

func (p *ConnPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var err error
	for addr, conn := range p.conns {
		if closeErr := conn.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		delete(p.conns, addr)
	}
	return err
}

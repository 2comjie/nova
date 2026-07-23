package client

import (
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type ConnPool struct {
	mu      sync.RWMutex
	connMap map[string]*grpc.ClientConn
	opts    []grpc.DialOption
}

func NewConnPool(opts ...grpc.DialOption) *ConnPool {
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	dialOpts = append(dialOpts, opts...)
	return &ConnPool{
		connMap: make(map[string]*grpc.ClientConn),
		opts:    dialOpts,
	}
}

func (p *ConnPool) Get(addr string) (*grpc.ClientConn, error) {
	if addr == "" {
		return nil, ErrInvalidTarget
	}
	p.mu.RLock()
	conn := p.connMap[addr]
	p.mu.RUnlock()
	if conn != nil {
		return conn, nil
	}

	conn, err := grpc.NewClient(addr, p.opts...)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	old := p.connMap[addr]
	if old != nil {
		p.mu.Unlock()
		_ = conn.Close()
		return old, nil
	}
	p.connMap[addr] = conn
	p.mu.Unlock()

	return conn, nil
}

func (p *ConnPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for addr, conn := range p.connMap {
		_ = conn.Close()
		delete(p.connMap, addr)
	}
}

func (p *ConnPool) Remove(addrs map[string]bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for addr := range addrs {
		if conn := p.connMap[addr]; conn != nil {
			_ = conn.Close()
			delete(p.connMap, addr)
		}
	}
}

func (p *ConnPool) Prune(activeAddrMap map[string]bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for addr, conn := range p.connMap {
		_, ok := activeAddrMap[addr]
		if !ok {
			_ = conn.Close()
			delete(p.connMap, addr)
		}
	}
}

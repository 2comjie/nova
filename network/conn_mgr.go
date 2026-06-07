package network

import (
	"sync"
	"sync/atomic"

	"github.com/2comjie/wali/core/help"
)

type BaseConnMgr struct {
	id         atomic.Int64
	total      atomic.Int64
	options    *Options
	partitions []*connPartition
}

func NewConnMgr(options *Options) *BaseConnMgr {
	cm := &BaseConnMgr{}
	cm.options = options
	cm.partitions = make([]*connPartition, 100)
	for i, _ := range cm.partitions {
		cm.partitions[i] = &connPartition{connections: make(map[int64]*BaseConn)}
	}
	return cm
}

func (cm *BaseConnMgr) Add(trans Transport) error {
	if cm.total.Load() >= cm.options.MaxConn {
		return ErrTooManyConn
	}
	id := cm.id.Add(1)
	conn := &BaseConn{}
	conn.init(cm, id, trans)
	cm.partitions[id%int64(len(cm.partitions))].store(id, conn)
	cm.total.Add(1)
	return nil
}

func (cm *BaseConnMgr) Close(reason string) {
	var wg sync.WaitGroup
	wg.Add(len(cm.partitions))
	for _, partition := range cm.partitions {
		help.SafeGo(func() {
			partition.close(reason)
			wg.Done()
		})
	}
	wg.Wait()
}

func (cm *BaseConnMgr) Stat() int64 {
	return cm.total.Load()
}

type connPartition struct {
	rw          sync.RWMutex
	connections map[int64]*BaseConn
}

func (p *connPartition) store(id int64, conn *BaseConn) {
	p.rw.Lock()
	p.connections[id] = conn
	p.rw.Unlock()
}

func (p *connPartition) delete(id int64) (*BaseConn, bool) {
	p.rw.Lock()
	conn, ok := p.connections[id]
	if ok {
		delete(p.connections, id)
	}
	p.rw.Unlock()
	return conn, ok
}

func (p *connPartition) close(reason string) {
	p.rw.Lock()
	for _, conn := range p.connections {
		_ = conn.Close(reason)
	}
	p.rw.Unlock()
}

package network

import (
	"hash/fnv"
	"sync"
	"sync/atomic"

	"github.com/2comjie/wali/core/help"
)

const partitionCount = 100

type BaseConnMgr struct {
	id         atomic.Int64
	total      atomic.Int64
	options    *Options
	partitions []*connPartition
	uidParts   []*uidPartition
}

func NewConnMgr(options *Options) *BaseConnMgr {
	cm := &BaseConnMgr{}
	cm.options = options
	cm.partitions = make([]*connPartition, partitionCount)
	for i := range cm.partitions {
		cm.partitions[i] = &connPartition{connections: make(map[int64]*BaseConn)}
	}
	cm.uidParts = make([]*uidPartition, partitionCount)
	for i := range cm.uidParts {
		cm.uidParts[i] = &uidPartition{index: make(map[string]*BaseConn)}
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
	cm.partitions[id%int64(partitionCount)].store(id, conn)
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

func (cm *BaseConnMgr) remove(id int64) {
	conn, ok := cm.partitions[id%int64(partitionCount)].delete(id)
	if !ok {
		return
	}
	cm.total.Add(-1)
	if uid := conn.UID(); uid != "" {
		cm.uidParts[cm.uidHash(uid)].delete(uid, id)
	}
}

func (cm *BaseConnMgr) Conn(id int64) (Conn, bool) {
	return cm.partitions[id%int64(partitionCount)].load(id)
}

func (cm *BaseConnMgr) ConnByUID(uid string) (Conn, bool) {
	if uid == "" {
		return nil, false
	}
	conn, ok := cm.uidParts[cm.uidHash(uid)].load(uid)
	if !ok || conn.State() == ConnClosed {
		return nil, false
	}
	return conn, true
}

func (cm *BaseConnMgr) BindUID(connID int64, uid string) error {
	if uid == "" {
		return ErrInvalidUID
	}

	cp := cm.partitions[connID%int64(partitionCount)]
	cp.rw.Lock()
	conn, ok := cp.connections[connID]
	if !ok {
		cp.rw.Unlock()
		return ErrConnNotFound
	}
	if conn.State() == ConnClosed {
		cp.rw.Unlock()
		return ErrConnClosed
	}

	oldUID := conn.UID()
	if oldUID == uid {
		// 快速路径: 同一个 UID, 检查是否已经正确映射
		up := cm.uidParts[cm.uidHash(uid)]
		up.rw.RLock()
		alreadyOK := up.index[uid] == conn
		up.rw.RUnlock()
		cp.rw.Unlock()
		if alreadyOK {
			return nil
		}
		// 映射被覆盖了, 需要修复, 走下面的逻辑重新绑定
	} else {
		cp.rw.Unlock()
	}

	// 清理旧 UID 映射
	if oldUID != "" {
		up := cm.uidParts[cm.uidHash(oldUID)]
		up.rw.Lock()
		if c := up.index[oldUID]; c != nil && c.ID() == connID {
			delete(up.index, oldUID)
		}
		up.rw.Unlock()
	}

	// 建立新 UID 映射
	up := cm.uidParts[cm.uidHash(uid)]
	up.rw.Lock()
	if oldConn := up.index[uid]; oldConn != nil && oldConn.ID() != connID {
		oldConn.setUID("") // 顶掉旧的连接
	}
	conn.setUID(uid)
	up.index[uid] = conn
	up.rw.Unlock()

	return nil
}

func (cm *BaseConnMgr) UnbindUID(connID int64) (string, error) {
	cp := cm.partitions[connID%int64(partitionCount)]
	cp.rw.Lock()
	conn, ok := cp.connections[connID]
	if !ok {
		cp.rw.Unlock()
		return "", ErrConnNotFound
	}
	uid := conn.UID()
	cp.rw.Unlock()

	if uid != "" {
		cm.uidParts[cm.uidHash(uid)].delete(uid, connID)
	}
	conn.setUID("")
	return uid, nil
}

func (cm *BaseConnMgr) Visit(fn func(Conn) bool) {
	for _, partition := range cm.partitions {
		if !partition.visit(fn) {
			return
		}
	}
}

func (cm *BaseConnMgr) Stat() int64 {
	return cm.total.Load()
}

func (cm *BaseConnMgr) uidHash(uid string) int {
	h := fnv.New64a()
	h.Write([]byte(uid))
	return int(h.Sum64() % partitionCount)
}

// ========= connPartition =========

type connPartition struct {
	rw          sync.RWMutex
	connections map[int64]*BaseConn
}

func (p *connPartition) store(id int64, conn *BaseConn) {
	p.rw.Lock()
	p.connections[id] = conn
	p.rw.Unlock()
}

func (p *connPartition) load(id int64) (*BaseConn, bool) {
	p.rw.RLock()
	conn, ok := p.connections[id]
	p.rw.RUnlock()
	return conn, ok
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

func (p *connPartition) visit(fn func(Conn) bool) bool {
	p.rw.RLock()
	conns := make([]Conn, 0, len(p.connections))
	for _, conn := range p.connections {
		conns = append(conns, conn)
	}
	p.rw.RUnlock()

	for _, conn := range conns {
		if !fn(conn) {
			return false
		}
	}
	return true
}

func (p *connPartition) close(reason string) {
	p.rw.Lock()
	for _, conn := range p.connections {
		_ = conn.Close(reason)
	}
	p.rw.Unlock()
}

// ========= uidPartition =========

type uidPartition struct {
	rw    sync.RWMutex
	index map[string]*BaseConn
}

func (p *uidPartition) load(uid string) (*BaseConn, bool) {
	p.rw.RLock()
	conn, ok := p.index[uid]
	p.rw.RUnlock()
	return conn, ok
}

func (p *uidPartition) delete(uid string, connID int64) bool {
	p.rw.Lock()
	defer p.rw.Unlock()
	conn, ok := p.index[uid]
	if ok && conn.ID() == connID {
		delete(p.index, uid)
		return true
	}
	return false
}

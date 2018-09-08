package tunnel

import (
	"sync"
)

type MPool struct {
	*sync.Pool
	sz int
}

func (p *MPool) Get() []byte {
	CT(T_Buf, OP_Increase)
	return p.Pool.Get().([]byte)
}

func (p *MPool) Put(x []byte) {
	if cap(x) == p.sz { //来自Pool的slice才可能回收.
		p.Pool.Put(x[0:p.sz])
		CT(T_Buf, OP_Decrease)
	}
}

func NewMPool(sz int) *MPool {
	p := &MPool{sz: sz}
	p.Pool = &sync.Pool{
		New: func() interface{} {
			return make([]byte, p.sz)
		},
	}
	return p
}

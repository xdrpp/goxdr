package rpc

import "sync"

type RmsgChanPool struct {
	pool sync.Pool
}

func NewRmsgChanPool() *RmsgChanPool {
	x := &RmsgChanPool{}
	x.pool.New = func() any {
		return make(chan *Rpc_msg, 1)
	}
	return x
}

func (x *RmsgChanPool) Get() chan *Rpc_msg {
	return x.pool.Get().(chan *Rpc_msg)
}

func (x *RmsgChanPool) Recycle(c chan *Rpc_msg) {
	select { // drain at most one message
	case <-c:
	default:
	}
	x.pool.Put(c)
}

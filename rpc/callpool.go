package rpc

import "github.com/xdrpp/goxdr/xdr"

type CallPool interface {
	NewCall() *PendingCall
	Recycle(*PendingCall)
	StatString() string
}

// arena

type callArena struct {
	arena *xdr.Arena[PendingCall]
}

func NewCallArena(cap int) CallPool {
	callArena := &callArena{}
	callArena.arena = xdr.NewArena(
		cap,
		func(p *PendingCall) {
			*p = PendingCall{}
		},
	)
	return callArena
}

func (callArena *callArena) StatString() string {
	return callArena.arena.StatString()
}

func (callArena *callArena) NewCall() *PendingCall {
	return callArena.arena.Get()
}

func (callArena *callArena) Recycle(call *PendingCall) {
	callArena.arena.Recycle(call)
}

// pool

type callPool struct {
	pool *xdr.Pool[PendingCall]
}

func NewCallPool() CallPool {
	x := &callPool{}
	x.pool = xdr.NewPool(
		func(call *PendingCall) {
			*call = PendingCall{}
		},
	)
	return x
}

func (callPool *callPool) NewCall() *PendingCall {
	return callPool.pool.Get()
}

func (callPool *callPool) Recycle(call *PendingCall) {
	callPool.pool.Recycle(call)
}

func (callPool *callPool) StatString() string {
	return callPool.pool.StatString()
}

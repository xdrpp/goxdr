package rpc

import (
	"github.com/xdrpp/goxdr/xdr"
)

type MessagePool interface {
	NewMessage(peer string) *Message
	Reycle(msg *Message)
	StatString() string
}

func NewMessagePool() MessagePool {
	return NewMsgPool(200)
	// return NewMsgArenaCap(5000)
}

// msg arena

type msgArena struct {
	arena *xdr.Arena[Message]
}

func NewMsgArenaCap(cap int) MessagePool {
	msgArena := &msgArena{}
	msgArena.arena = xdr.NewArena(
		cap,
		func(m *Message) {
			m.pool = msgArena
			m.Peer = ""
			m.Buffer.Reset()
		},
	)
	return msgArena
}

func (msgArena *msgArena) StatString() string {
	return msgArena.arena.StatString()
}

func (msgArena *msgArena) NewMessage(peer string) *Message {
	msg := msgArena.arena.Get()
	msg.Peer = peer
	return msg
}

func (msgArena *msgArena) Reycle(msg *Message) {
	msgArena.arena.Recycle(msg)
}

// msg pool

type msgPool struct {
	pool *xdr.Pool[Message]
}

func NewMsgPool(preAlloc int) MessagePool {
	x := &msgPool{}
	x.pool = xdr.NewPool(
		func(m *Message) {
			m.pool = x
			m.Peer = ""
			if m.Buffer.Cap() == 0 {
				m.Buffer.Grow(preAlloc)
			}
			m.Buffer.Reset()
		},
	)
	return x
}

func (msgPool *msgPool) NewMessage(peer string) *Message {
	msg := msgPool.pool.Get()
	msg.Peer = peer
	return msg
}

func (msgPool *msgPool) Reycle(msg *Message) {
	msgPool.pool.Recycle(msg)
}

func (msgPool *msgPool) StatString() string {
	return msgPool.pool.StatString()
}

package rpc

import (
	"time"

	"github.com/xdrpp/goxdr/xdr"
)

type MessagePool interface {
	NewMessage(peer string) *Message
	Recycle(msg *Message)
	StatString() string
}

// msg arena

type msgArena struct {
	arena *xdr.Arena[Message]
}

var zeroTime time.Time

func NewMsgArena(cap int) MessagePool {
	msgArena := &msgArena{}
	msgArena.arena = xdr.NewArena(
		cap,
		func(m *Message) {
			m.pool = msgArena
			m.Peer = ""
			m.Buffer.Reset()
			m.enteredQueue = zeroTime
			m.ioLatency, m.queueLatency, m.serdeLatency = 0, 0, 0
			m.report = nil
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

func (msgArena *msgArena) Recycle(msg *Message) {
	msgArena.arena.Recycle(msg)
}

// msg pool

type msgPool struct {
	pool *xdr.Pool[Message]
}

func NewMsgPool() MessagePool {
	x := &msgPool{}
	x.pool = xdr.NewPool(
		func(m *Message) {
			m.pool = x
			m.Peer = ""
			m.Buffer.Reset()
			m.enteredQueue = zeroTime
			m.ioLatency, m.queueLatency, m.serdeLatency = 0, 0, 0
			m.report = nil
		},
	)
	return x
}

func (msgPool *msgPool) NewMessage(peer string) *Message {
	msg := msgPool.pool.Get()
	msg.Peer = peer
	return msg
}

func (msgPool *msgPool) Recycle(msg *Message) {
	msgPool.pool.Recycle(msg)
}

func (msgPool *msgPool) StatString() string {
	return msgPool.pool.StatString()
}

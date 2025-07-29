package rpc

import "github.com/xdrpp/goxdr/xdr"

type MessagePool interface {
	NewMessage(peer string) *Message
	Reycle(msg *Message)
	StatString() string
}

type msgArena struct {
	arena *xdr.Arena[Message]
}

func NewMsgArena() MessagePool {
	return NewMsgArenaCap(5000)
}

func NewMsgArenaCap(cap int) MessagePool {
	msgPool := &msgArena{}
	msgPool.arena = xdr.NewArena(
		cap,
		func(m *Message) {
			m.pool = msgPool
			m.Peer = ""
			m.Buffer.Reset()
		},
	)
	return msgPool
}

func (msgPool *msgArena) StatString() string {
	return msgPool.arena.StatString()
}

func (msgPool *msgArena) NewMessage(peer string) *Message {
	msg := msgPool.arena.Get()
	msg.Peer = peer
	return msg
}

func (msgPool *msgArena) Reycle(msg *Message) {
	msgPool.arena.Recycle(msg)
}

/*
type MsgPool xdr.Pool[*Message]

func NewMsgPool() *MsgPool {
	pool := &xdr.Pool[*Message]{}
	pool.SetMkReset(
		func() *Message {
			return &Message{Buffer: &bytes.Buffer{}, pool: (*MsgPool)(pool)}
		},
		func(m *Message) {
			m.Peer = ""
			m.Buffer.Reset()
		},
	)
	return (*MsgPool)(pool)
}

func (msgPool *MsgPool) NewMessage(peer string) *Message {
	msg := (*xdr.Pool[*Message])(msgPool).Get()
	msg.Peer = peer
	return msg
}

func (msgPool *MsgPool) Reycle(msg *Message) {
	(*xdr.Pool[*Message])(msgPool).Recycle(msg)
}
*/

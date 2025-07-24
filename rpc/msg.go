package rpc

import (
	"bytes"

	"github.com/xdrpp/goxdr/xdr"
)

type Message struct {
	*bytes.Buffer
	Peer string
	pool *MsgPool
}

type MsgPool struct {
	arena *xdr.Arena[Message]
}

func NewMsgPool() *MsgPool {
	return NewMsgPoolCap(10000)
}

func NewMsgPoolCap(cap int) *MsgPool {
	msgPool := &MsgPool{}
	msgPool.arena = xdr.NewArena(
		cap,
		func(m *Message) {
			m.Buffer = &bytes.Buffer{}
			m.pool = msgPool
		},
		func(m *Message) {
			m.Peer = ""
			m.Buffer.Reset()
		},
	)
	return msgPool
}

func (msgPool *MsgPool) NewMessage(peer string) *Message {
	msg := msgPool.arena.Get()
	msg.Peer = peer
	return msg
}

func (msgPool *MsgPool) Reycle(msg *Message) {
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

func (m *Message) Recycle() {
	if m == nil {
		panic("XXX m=nil")
	}
	m.pool.Reycle(m)
}

func (m *Message) In() xdr.XDR {
	return xdr.XdrIn{In: m}
}

func (m *Message) Out() xdr.XDR {
	return xdr.XdrOut{Out: m}
}

func (m *Message) Xid() uint32 {
	if bs := m.Bytes(); len(bs) >= 4 {
		return uint32(bs[0])<<24 | uint32(bs[1])<<16 |
			uint32(bs[2])<<8 | uint32(bs[3])
	}
	return 0
}

func (m *Message) Serialize(vs ...xdr.XdrType) {
	out := m.Out()
	for i := range vs {
		vs[i].XdrMarshal(out, "")
	}
}

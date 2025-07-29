package rpc

import (
	"bytes"

	"github.com/xdrpp/goxdr/xdr"
)

type Message struct {
	bytes.Buffer
	Peer string
	pool MessagePool
}

func (m *Message) Recycle() {
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

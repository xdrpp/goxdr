
package rpc

import (
	"github.com/xdrpp/goxdr/xdr"
)

type CallKey struct {
	Xid uint32
	Peer string
}

// An RPC message decoder that relies on XdrCatalog and keeps track of
// XID values to may replies to calls.  Primarily intended for
// analyzing RPC traces, since it is not connected to any
// implementations of RPC interfaces.
type RpcDecoder struct {
	Calls map[CallKey]xdr.XdrProc
}

// Return decoded calls and replies from a trace.  The call types are
// looked up based on the Prog, Vers, and Proc in the message header.
// Reply types are taken from a cache based on the calls.  If the type
// of the argument or result cannot be found, or unmarshalling fails,
// returns a nil XdrProc.  If there is a problem with the message
// header, returns a nil *Rpc_msg.
func (d *RpcDecoder) Decode(m *Message) (*Rpc_msg, xdr.XdrProc) {
	msg, err := GetMsg(m.In())
	if err != nil {
		return nil, nil
	}
	ck := CallKey {
		Xid: msg.Xid,
		Peer: m.Peer,
	}
	if msg.Body.Mtype == REPLY {
		if p, ok := d.Calls[ck]; ok {
			delete(d.Calls, ck)
			if safeMarshal(m.In(), p.GetRes(), "") != nil {
				return msg, nil
			}
			return msg, p
		}
	} else if msg.Body.Mtype != CALL {
		return msg, nil
	}
	var p xdr.XdrProc
	if f := xdr.XdrCatalog[uint64(msg.Body.Cbody().Prog)<<32|
		uint64(msg.Body.Cbody().Vers)]; f != nil {
		p = f(msg.Body.Cbody().Proc)
	}
	if p != nil {
		if safeMarshal(m.In(), p.GetArg(), "") != nil {
			p = nil
		}
	}
	return msg, p
}

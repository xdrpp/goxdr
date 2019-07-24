
// Utilities for implementing RFC5531 RPC.
package rpcmsg

import (
	"fmt"
	"github.com/xdrpp/goxdr/xdr"
)

// Fake accept state to represent a response we can't unmarshal
const GARBAGE_RET Accept_stat = 99
func init() {
	_XdrNames_Accept_stat[int32(GARBAGE_RET)] = "GARBAGE_RET"
	_XdrValues_Accept_stat["GARBAGE_RET"] = int32(GARBAGE_RET)
	_XdrComments_Accept_stat[int32(GARBAGE_RET)] =
		"Unable to decode return value"
}

func safeMarshal(x xdr.XDR, t xdr.XdrType, name string) (err error) {
	defer func() {
		if i := recover(); i != nil {
			if e, ok := i.(xdr.XdrError); ok {
				err = e
			} else {
				panic(i)
			}
		}
	}()
	t.XdrMarshal(x, name)
	return nil
}

// Returns true iff rmsg is an accepted REPLY with status SUCCESS.
func IsSuccess(rmsg *Rpc_msg) bool {
	return rmsg != nil &&
		rmsg.Body.Mtype == REPLY &&
		rmsg.Body.Rbody().Stat == MSG_ACCEPTED &&
		rmsg.Body.Rbody().Areply().Reply_data.Stat == SUCCESS
}

// Unmarshal an Rpc_msg header, returning an error instead of throwing
// an exception if the bytes are garbage.
func GetMsg(in xdr.XDR) (*Rpc_msg, error) {
	var m Rpc_msg
	if err := safeMarshal(in, &m, ""); err != nil {
		return nil, err
	}
	return &m, nil
}

type PendingCall struct {
	Proc xdr.XdrProc
	Cb func(*Rpc_msg)
}

type CallSet struct {
	LastXid uint32
	Calls map[uint32]*PendingCall
}

// Allocate a new XID and create a message header for an outgoing RPC
// call.
func (cs *CallSet) NewCall(proc xdr.XdrProc, cb func(*Rpc_msg)) *Rpc_msg {
	if cs.Calls == nil {
		cs.Calls = make(map[uint32]*PendingCall)
	}
	for ok := true; ok; {
		cs.LastXid++
		_, ok = cs.Calls[cs.LastXid]
	}
	cs.Calls[cs.LastXid] = &PendingCall{
		Proc: proc,
		Cb: cb,
	}
	cmsg := Rpc_msg { Xid: cs.LastXid }
	cmsg.Body.Mtype = CALL
	cmsg.Body.Cbody().Rpcvers = 2
	cmsg.Body.Cbody().Prog = proc.Prog()
	cmsg.Body.Cbody().Vers = proc.Vers()
	cmsg.Body.Cbody().Proc = proc.Proc()
	return &cmsg
}

// Free the XID associated with a pending RPC call if the call is
// being canceled.
func (cs *CallSet) Free(msg *Rpc_msg) {
	delete(cs.Calls, msg.Xid)
}

// Attempt to match a reply with a pending call, and make the callback
// if it succeeds.
func (cs *CallSet) GetReply(rmsg *Rpc_msg, in xdr.XDR) {
	if rmsg.Body.Mtype != REPLY {
		return
	}
	pc, ok := cs.Calls[rmsg.Xid]
	if !ok {
		return
	}
	cs.Free(rmsg)
	if IsSuccess(rmsg) {
		if err := safeMarshal(in, pc.Proc.GetRes(), ""); err != nil {
			rmsg.Body.Rbody().Areply().Reply_data.Stat = GARBAGE_RET
		}
	}
	pc.Cb(rmsg)
}

// Container for a server implementing a set of program/version
// interfaces.
type RpcSrv struct {
	Srvs map[uint32]map[uint32]xdr.XdrSrv
}

// Register a service that receives RPC calls.
func (s *RpcSrv) Register(srv xdr.XdrSrv) {
	if s.Srvs == nil {
		s.Srvs = make(map[uint32]map[uint32]xdr.XdrSrv)
	}
	prog := s.Srvs[srv.Prog()]
	if prog == nil {
		prog = make(map[uint32]xdr.XdrSrv)
		s.Srvs[srv.Prog()] = prog
	}
	prog[srv.Vers()] = srv
}

// Receive an RPC call in a server, and format the reply header.  Does
// not actually perform the requested remote procedure call.  To
// perform the call, you should call proc.Do() (maybe in a goroutine)
// when this function returns a non-NULL proc.
//
// If in == nil, then this function skips unmarshalling the arguments.
func (s RpcSrv) GetProc(cmsg *Rpc_msg, in xdr.XDR) (
	rmsg *Rpc_msg, proc xdr.XdrSrvProc) {
	if cmsg.Body.Mtype != CALL {
		return nil, nil
	}
	rmsg = &Rpc_msg { Xid: cmsg.Xid }
	rmsg.Body.Mtype = REPLY

	if cmsg.Body.Cbody().Rpcvers != 2 {
		rmsg.Body.Rbody().Stat = MSG_DENIED
		rmsg.Body.Rbody().Rreply().Stat = RPC_MISMATCH
		rmsg.Body.Rbody().Rreply().Mismatch_info().Low = 2
		rmsg.Body.Rbody().Rreply().Mismatch_info().High = 2
		return
	}
	rmsg.Body.Rbody().Stat = MSG_ACCEPTED

	if prog, ok := s.Srvs[cmsg.Body.Cbody().Prog]; !ok {
		rmsg.Body.Rbody().Areply().Reply_data.Stat = PROG_UNAVAIL
	} else if vers, ok := prog[cmsg.Body.Cbody().Vers]; !ok {
		rmsg.Body.Rbody().Areply().Reply_data.Stat = PROG_MISMATCH
		mmi := rmsg.Body.Rbody().Areply().Reply_data.Mismatch_info()
		mmi.Low = 0xffffffff
		for i := range prog {
			if i < mmi.Low {
				mmi.Low = i
			}
			if i > mmi.High {
				mmi.High = i
			}
		}
	} else if proc = vers.GetProc(cmsg.Body.Cbody().Proc); proc == nil {
		rmsg.Body.Rbody().Areply().Reply_data.Stat = PROC_UNAVAIL
	} else {
		rmsg.Body.Rbody().Areply().Reply_data.Stat = SUCCESS
		if in != nil && safeMarshal(in, proc.GetArg(), "") != nil {
			rmsg.Body.Rbody().Areply().Reply_data.Stat = GARBAGE_ARGS
		}
	}

	return
}

// An *Rpc_msg can represent an error.  Call IsSuccess to see if there
// was actually an error.
func (m Rpc_msg) Error() string {
	if m.Body.Mtype != REPLY {
		return "RPC message not a REPLY"
	} else if m.Body.Rbody().Stat == MSG_ACCEPTED {
		stat := m.Body.Rbody().Areply().Reply_data.Stat
		c, ok := stat.XdrEnumComments()[int32(stat)]
		if !ok {
			c = stat.String()
		}
		if stat == PROG_MISMATCH {
			mmi := m.Body.Rbody().Areply().Reply_data.Mismatch_info()
			c = fmt.Sprintf("%s (low %d, high %d)", c, mmi.Low, mmi.High)
		}
		return c
	} else if m.Body.Rbody().Stat == MSG_DENIED {
		stat := m.Body.Rbody().Rreply().Stat
		c, ok := stat.XdrEnumComments()[int32(stat)]
		if !ok {
			c = stat.String()
		}
		return c
	}
	return "Invalid reply_stat"
}

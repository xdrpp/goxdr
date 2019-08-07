
// Utilities for implementing RFC5531 RPC.
package rpc

import (
	"fmt"
	"github.com/xdrpp/goxdr/xdr"
	"sync"
)

// Fake accept state to represent a cancelled call
const CANCELED Accept_stat = 98
// Fake accept state to represent a response we can't unmarshal
const GARBAGE_RET Accept_stat = 99
func init() {
	pseudo_states := []struct{stat Accept_stat; name string; comment string}{
		{CANCELED, "CANCELED", "Call context canceled"},
		{GARBAGE_RET, "GARBAGE_RET", "Unable to decode return value"},
	}
	for _, ps := range pseudo_states {
		_XdrNames_Accept_stat[int32(ps.stat)] = ps.name
		_XdrValues_Accept_stat[ps.name] = int32(ps.stat)
		_XdrComments_Accept_stat[int32(ps.stat)] = ps.comment
	}
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
	return
}

// Sets an rpc_message to be an accepted reply with a particular
// status.
func SetStat(msg *Rpc_msg, stat Accept_stat) {
	msg.Body.Mtype = REPLY
	msg.Body.Rbody().Stat = MSG_ACCEPTED
	msg.Body.Rbody().Areply().Reply_data.Stat = stat
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
	Server string
}

// A CallSet represents a set of pending calls.  Its methods can be
// called concurrently from multiple threads.
type CallSet struct {
	lock sync.Mutex
	lastXid uint32
	calls map[uint32]*PendingCall
}

// Allocate a new XID and create a message header for an outgoing RPC
// call.  The server string is just an arbitrary name for the server
// to avoid interpreting messages from one server as replies to RPC
// sent to a different server.  If the CallSet is only used for one
// server, the server string can always be empty.
func (cs *CallSet) NewCall(server string, proc xdr.XdrProc,
	cb func(*Rpc_msg)) *Rpc_msg {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	if cs.calls == nil {
		cs.calls = make(map[uint32]*PendingCall)
	}
	for ok := true; ok; {
		cs.lastXid++
		_, ok = cs.calls[cs.lastXid]
	}
	cs.calls[cs.lastXid] = &PendingCall{
		Proc: proc,
		Cb: cb,
		Server: server,
	}
	cmsg := Rpc_msg { Xid: cs.lastXid }
	cmsg.Body.Mtype = CALL
	cmsg.Body.Cbody().Rpcvers = 2
	cmsg.Body.Cbody().Prog = proc.Prog()
	cmsg.Body.Cbody().Vers = proc.Vers()
	cmsg.Body.Cbody().Proc = proc.Proc()
	return &cmsg
}

// Delete a pending call without making its callback.
func (cs *CallSet) Delete(xid uint32) {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	delete(cs.calls, xid)
}

// Cancel a pending call and return a fake Rpc_msg with the the
// CANCELED pseudo-error.
func (cs *CallSet) Cancel(xid uint32) {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	if pc, ok := cs.calls[xid]; ok {
		delete(cs.calls, xid)
		rmsg := Rpc_msg{ Xid: xid }
		SetStat(&rmsg, CANCELED)
		pc.Cb(&rmsg)
	}
}

func (cs *CallSet) CancelAll() {
	cs.lock.Lock()
	defer cs.lock.Unlock()
	calls := cs.calls
	cs.calls = nil
	for xid, pc := range calls {
		rmsg := Rpc_msg{ Xid: xid }
		SetStat(&rmsg, CANCELED)
		pc.Cb(&rmsg)
	}
}

// Attempt to match a reply with a pending call.  If it returns
// a non-nil pc, you should call pc.Cb(rmsg).
func (cs *CallSet) GetReply(server string, rmsg *Rpc_msg,
	in xdr.XDR) *PendingCall {
	if rmsg.Body.Mtype != REPLY {
		return nil
	}
	cs.lock.Lock()
	defer cs.lock.Unlock()
	pc, ok := cs.calls[rmsg.Xid]
	if !ok || pc.Server != server {
		return nil
	}
	delete(cs.calls, rmsg.Xid)
	if IsSuccess(rmsg) {
		if err := safeMarshal(in, pc.Proc.GetRes(), ""); err != nil {
			rmsg.Body.Rbody().Areply().Reply_data.Stat = GARBAGE_RET
		}
	}
	return pc
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

	if cmsg.Body.Cbody().Rpcvers != 2 {
		rmsg.Body.Mtype = REPLY
		rmsg.Body.Rbody().Stat = MSG_DENIED
		rmsg.Body.Rbody().Rreply().Stat = RPC_MISMATCH
		rmsg.Body.Rbody().Rreply().Mismatch_info().Low = 2
		rmsg.Body.Rbody().Rreply().Mismatch_info().High = 2
		return
	}

	if prog, ok := s.Srvs[cmsg.Body.Cbody().Prog]; !ok {
		SetStat(rmsg, PROG_UNAVAIL)
	} else if vers, ok := prog[cmsg.Body.Cbody().Vers]; !ok {
		SetStat(rmsg, PROG_MISMATCH)
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
		SetStat(rmsg, PROC_UNAVAIL)
	} else if in != nil && safeMarshal(in, proc.GetArg(), "") != nil {
		proc = nil
		SetStat(rmsg, GARBAGE_ARGS)
	} else {
		SetStat(rmsg, SUCCESS)
	}

	return
}

// An *Rpc_msg can represent an error.  Call IsSuccess to see if there
// was actually an error.
func (m *Rpc_msg) Error() string {
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


// Utilities for implementing RFC5531 RPC.
package rpcmsg

import "github.com/xdrpp/goxdr/xdr"

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

// Unmarshal an Rpc_msg header, returning an error instead of throwing
// an exception if the bytes are garbage.
func GetMsg(in xdr.XDR) (*Rpc_msg, error) {
	var m Rpc_msg
	if err := safeMarshal(in, &m, ""); err != nil {
		return nil, err
	}
	return &m, nil
}

// Container for a bunch of XdrSrv structures.
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
// not actually execute the call (which you would do by calling
// proc.Do() when this function returns a non-NULL proc).
//
// If cmsg == nil, will read the call header directly from in.  The
// reason you might not want to do this if you are multiplexing calls
// and replies on the same underlying transport, in which case you
// will first want to get the header to check if you have just
// received a call or a reply.
//
// If in == nil (in which case cmsg may not be nil), then this
// function will skip unmarshalling the arguments.
func (s RpcSrv) GetProc(cmsg *Rpc_msg, in xdr.XDR) (
	rmsg *Rpc_msg, proc xdr.XdrSrvProc) {
	if cmsg == nil {
		var err error
		if cmsg, err = GetMsg(in); err != nil {
			return nil, nil
		}
	}

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


package rpc

import (
	"context"
	"fmt"
	"github.com/xdrpp/goxdr/xdr"
	"os"
)

type peerKeyType struct {}
var peerKey peerKeyType

// Get the network address associated with a context.
func GetPeer(ctx context.Context) string {
	if ctx != nil {
		if peer, ok := ctx.Value(peerKey).(string); ok {
			return peer
		}
	}
	return ""
}

// Associate the network address of a peer with a context.
func WithPeer(ctx context.Context, peer string) context.Context {
	if peer == GetPeer(ctx) {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, peerKey, peer)
}

// Return true if ctx is a non-nil context that is done.
func IsDone(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// Create a channel that return received messages from a Transport.
func ReceiveChan(ctx context.Context, t Transport) <-chan *Message {
	ret := make(chan *Message)
	go func(c chan<- *Message) {
		for {
			m, err := t.Receive()
			if err != nil {
				close(c)
				return
			}
			select {
			case c <- m:
			case <-ctx.Done():
				close(c)
				return
			}
		}
	}(ret)
	return ret
}

// Create a channel for sending messages through a Transport.
func SendChan(t Transport) chan<- *Message {
	ret := make(chan *Message, 1)
	go func(c <-chan *Message) {
		for {
			if m, ok := <-c; !ok {
				return
			} else {
				t.Send(m)
			}
		}
	}(ret)
	return ret
}


// Simple RPC driver.  Can handle calls from multiple threads, but all
// server side handling happens in a single thread (inside Loop()).
type RPC struct {
	Srv RpcSrv
	ctx context.Context
	cancel context.CancelFunc
	out chan<- *Message
	in <-chan *Message
	cs CallSet
}

func NewRpc(ctx context.Context, t Transport) *RPC {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	ret := RPC {
		ctx: ctx,
		cancel: cancel,
		out: SendChan(t),
		in: ReceiveChan(ctx, t),
	}
	go func() {
		<-ctx.Done()
		t.Close()
		close(ret.out)
	}()

	return &ret
}

func (r *RPC) Close() {
	r.cancel()
}

func (r *RPC) safeSend(ctx context.Context, m *Message) (ok bool) {
	defer func() { recover() }()
	select {
	case r.out <- m:
		return true
	case <-ctx.Done():
		return false
	}
}

func (r *RPC) SendCall(ctx context.Context, proc xdr.XdrProc) (err error) {
	c := make(chan *Rpc_msg, 1)
	peer := GetPeer(ctx)
	cmsg := r.cs.NewCall(peer, proc, func(rmsg *Rpc_msg) {
		c <- rmsg
		close(c)
	})
	m := Message { Peer: GetPeer(ctx) }
	m.Serialize(cmsg, proc.GetArg())
	if !r.safeSend(ctx, &m) {
		r.cs.Delete(cmsg.Xid)
		return ErrTransportClosed
	}
	select {
	case rmsg := <-c:
		if IsSuccess(rmsg) {
			return nil
		}
		return rmsg
	case <-ctx.Done():
		// XXX for reliable transport, should technically keep the XID
		// around to avoid recycling it before the server replies.
		// (This is also why channel c has a buffer size of 1.)
		r.cs.Delete(cmsg.Xid)
		return ErrTransportClosed
	}
}

// Call this once in the thread that handles all incoming calls.  If
// there are no services registered, then just call it in a separate
// gorouting to handle replies to outgoing calls.
func (r *RPC) Loop() {
	for {
		m := <-r.in
		if m == nil {
			break
		}
		msg, err := GetMsg(m.In())
		if err != nil {
			fmt.Println(os.Stderr, err)
			break
		}

		// Only has effect if we just received a call
		r.cs.GetReply(m.Peer, msg, m.In())

		// Only has effect if we just received a reply
		if rmsg, proc := r.Srv.GetProc(msg, m.In()); rmsg != nil {
			reply := Message{ Peer: m.Peer }
			reply.Serialize(rmsg)
			if proc != nil {
				ctx := r.ctx
				if reply.Peer != GetPeer(ctx) {
					ctx = WithPeer(ctx, m.Peer)
				}
				if ctx != nil {
					proc.SetContext(ctx)
				}
				proc.Do()
				reply.Serialize(proc.GetRes())
			}
			r.safeSend(r.ctx, &reply)
		}
	}
	if !IsDone(r.ctx) {
		r.Close()
	}
	r.cs.CancelAll()
}

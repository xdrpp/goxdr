
package rpc

import (
	"bytes"
	"context"
	"fmt"
	"github.com/xdrpp/goxdr/xdr"
	"io"
	"os"
	"sync"
	"sync/atomic"
)

// The default value for Log in newly allocated RPCs.
var DefaultLog io.Writer

// We have two types of value we can associate with a context, one
// that just has a peer address string, and one that has both a peer
// address and a way to Detach a server call.
type ctxKeyType struct{}
var ctxKey ctxKeyType

type peerCtxVal interface {
	GetPeer() string
	WithPeer(string) peerCtxVal
}
type peerCtx struct {
	Peer string
}
var _ peerCtxVal = &peerCtx{}	// XXX
func (v *peerCtx) GetPeer() string {
	return v.Peer
}
func (v *peerCtx) WithPeer(peer string) peerCtxVal {
	return &peerCtx{ Peer: peer }
}

// Get the network address associated with a context.
func GetPeer(ctx context.Context) string {
	if ctx != nil {
		if pc, ok := ctx.Value(ctxKey).(peerCtxVal); ok {
			return pc.GetPeer()
		}
	}
	return ""
}

// Associate the network address of a peer with a context.
func WithPeer(ctx context.Context, peer string) context.Context {
	pc, ok := ctx.Value(ctxKey).(peerCtxVal)
	if ok {
		if pc.GetPeer() == peer {
			return ctx
		}
		pc = pc.WithPeer(peer)
	} else {
		pc = &peerCtx{ Peer: peer }
	}
	return context.WithValue(ctx, ctxKey, pc)
}

type srvCtxVal interface {
	peerCtxVal
	Detach()
}
type srvCtx struct {
	peerCtx
	unlock func()
}
var _ srvCtxVal = &srvCtx{}		// XXX
func (v *srvCtx) WithPeer(peer string) peerCtxVal {
	ret := *v
	ret.Peer = peer
	return &ret
}
func (v *srvCtx) Detach() {
	v.unlock()
}

// By default, a Driver will only service one incoming call at a time.
// Calling Detach on the Context from within server-side code for a
// call will allow the Loop thread to service another call before the
// first call replies.
func Detach(ctx context.Context) {
	if sc, ok := ctx.Value(ctxKey).(srvCtxVal); ok {
		sc.Detach()
	}
}

// Create a channel that return received messages from a Transport.
// Creates a thread that closes the channel and exits when the Context
// is Done or when the transport returns an error.  Does not close the
// Transport.
func ReceiveChan(ctx context.Context, t Transport) <-chan *Message {
	ret := make(chan *Message)
	go func(c chan<- *Message) {
		for {
			m, err := t.Receive()
			if err != nil {
				if err != io.EOF {
					fmt.Fprintf(os.Stderr, "ReceiveChan: %s\n", err)
				}
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

// Create a channel for sending messages through a Transport.  Creates
// a thread that won't exit until the returned channel is closed.
// Does not close the underlying Transport.
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

// RPC driver implements all Transport-agnostic logic for handling
// incoming and outgoing RPCs.
//
// You must create a Driver with NewDriver().  A new Driver will not
// process messages until you call the Go() method.  On a server, you
// will often want to call Go() in the main thread.  In a client, you
// will need to invoke Go it in its own goroutine.
//
// Between the time you create a Driver and the time you call its Go()
// method, you may want to do any of the following to customize the
// driver:
//
// * On a server, register services that you are providing by calling
// the Register() method.
//
// * Set the Lock field to allow more or less concurrent handling of
// incoming requests.
//
// * Set the Log field to a non-nil io.Writer.
type Driver struct {
	// Lock preventing concurrent handling of incoming RPCs on a
	// server.  By default, this field is set to a new sync.Mutex,
	// meaning that only one call at a time is handled.  A long
	// running call can decide to complete asynchronously, however, by
	// calling Detach() its context, which immediately releases Lock
	// and allows the next call to be dispatched in parallel.
	//
	// To process all calls in parallel, you can set Lock to nil.
	//
	// Conversely, to serialize calls on multiple drivers, they can
	// all share the same Lock.
	//
	// Never release this lock from within a server-side RPC routine,
	// as otherwise the same thread will release it a second time when
	// you return from the RPC.  Instead, use Detach() to free the
	// lock, as this ensures Lock will only be freed once.
	Lock sync.Locker

	// If set to non-nil, a human-readable trace of all incoming and
	// outgoing RPCs will be written there.  Set the global DefaultLog
	// if you want to trace all Drivers by default.
	Log io.Writer

	srv RpcSrv
	ctx context.Context
	cancel context.CancelFunc
	out chan<- *Message
	in <-chan *Message
	cs CallSet
	started int32
}

func (r *Driver) logXdr(t xdr.XdrType, f string, args...interface{}) {
	if r.Log == nil {
		return
	}
	var out bytes.Buffer
	fmt.Fprintf(&out, f, args...)
	out.WriteByte('\n')
	t.XdrMarshal(xdr.XdrPrint{&out}, "")
	r.Log.Write(out.Bytes())
}

// Allocate a Driver for a transport.  NewDriver takes ownership of
// the Transport it and will Close it when Done.  Do not use a
// Transport after you have passed it to Driver.
//
// The only way to free a Driver is to cancel the Context ctx that you
// have passed in.  If you will never need to free the Driver, you may
// supply a nil ctx.
func NewDriver(ctx context.Context, t Transport) *Driver {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	ret := Driver {
		Log: DefaultLog,
		Lock: &sync.Mutex{},
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

// Register an RPC server.
func (r *Driver) Register(srv xdr.XdrSrv) {
	r.srv.Register(srv)
}

// Free a Driver, close its transport, and cancel any pending calls
// (which will return with errors).  Calling this is the same as
// canceling the context that was passed to NewDriver.
func (r *Driver) Close() {
	r.cancel()
}

func (r *Driver) safeSend(ctx context.Context, m *Message) (ok bool) {
	defer func() { recover() }()
	select {
	case r.out <- m:
		return true
	case <-ctx.Done():
		return false
	}
}

// Driver implements the XdrSendCall interface, allowing it to be
// used as the Send field of generated RPC client structures.
func (r *Driver) SendCall(ctx context.Context, proc xdr.XdrProc) (err error) {
	c := make(chan *Rpc_msg, 1)
	peer := GetPeer(ctx)
	cmsg := r.cs.NewCall(peer, proc, func(rmsg *Rpc_msg) {
		c <- rmsg
		close(c)
	})
	m := Message { Peer: peer }
	r.logXdr(proc.GetArg(), "->%s CALL(xid=%d) %s", peer, cmsg.Xid,
		proc.ProcName())
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
		// The fact that we don't read from c here is why it needs a
		// buffer size of 1.
		//
		// XXX - for reliable transport, should technically keep the
		// XID around to avoid recycling it before the server replies.
		r.cs.Delete(cmsg.Xid)
		return ErrTransportClosed
	}
}

// Acquire a lock and return an idempotent unlock function.
func mkUnlocker(lock sync.Locker) func() {
	if lock == nil {
		return func(){}
	}
	once := sync.Once{}
	lock.Lock()
	return func() { once.Do(lock.Unlock) }
}

// The main loop for handling incoming requests.  On a server, you
// will likely want to call this in the main thread after calling
// Register on one or more services.  On a client, you will want to
// start this in its own goroutine to handle incoming replies.
func (r *Driver) Go() {
	if atomic.SwapInt32(&r.started, 1) == 1 {
		panic("rpc.Driver.Go called multiple times")
	}
	for {
		m := <-r.in
		if m == nil {
			break
		}
		msg, err := GetMsg(m.In())
		if err != nil {
			fmt.Fprintf(os.Stderr, "GetMsg failed: %s\n", err)
			break
		}

		if pc := r.cs.GetReply(m.Peer, msg, m.In()); pc != nil {
			r.logXdr(pc.Proc.GetRes(), "<-%s REPLY(xid=%d) %s",
				m.Peer, msg.Xid, pc.Proc.ProcName())
			pc.Cb(msg)
			continue
		}

		rmsg, proc := r.srv.GetProc(msg, m.In())
		if rmsg == nil {
			continue
		} else if proc == nil {
			reply := Message{ Peer: m.Peer }
			reply.Serialize(rmsg)
			continue
		}

		unlock := mkUnlocker(r.Lock)
		r.logXdr(proc.GetArg(), "<-%s CALL(xid=%d) %s",
			m.Peer, msg.Xid, proc.ProcName())
		proc.SetContext(context.WithValue(r.ctx, ctxKey, &srvCtx {
			peerCtx: peerCtx{ Peer: m.Peer },
			unlock: unlock,
		}))

		go func() {
			defer func() {
				unlock()
				if i := recover(); i != nil {
					fmt.Fprintf(os.Stderr, "%s\n", i)
					if IsSuccess(rmsg) {
						SetStat(rmsg, SYSTEM_ERR)
					}
				}
				reply := Message{ Peer: m.Peer }
				reply.Serialize(rmsg)
				if IsSuccess(rmsg) {
					reply.Serialize(proc.GetRes())
					r.logXdr(proc.GetRes(), "->%s REPLY(xid=%d) %s",
						m.Peer, msg.Xid, proc.ProcName())
				}
				r.safeSend(r.ctx, &reply)
			}()
			proc.Do()
		}()
	}
	r.Close()
	r.cs.CancelAll()
}

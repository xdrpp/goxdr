package rpc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/xdrpp/goxdr/xdr"
)

type Message struct {
	*bytes.Buffer
	Peer    string
	reclaim func()
}

func NewMessage(peer string, buf *bytes.Buffer, reclaim func()) *Message {
	return &Message{Buffer: buf, Peer: peer, reclaim: reclaim}
}

func (m *Message) Reclaim() {
	m.reclaim()
	m.reclaim = nil
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

// Abstraction for an RPC transport.  Note that Send() should not be
// called multiple times concurrently and Receive() should not be
// called multiple times concurrently, but it is okay to call Send()
// concurrently with Receive().
type Transport interface {
	// Send a message.
	Send(*Message) error

	// Receive the next incoming message.  Blocks until a message is
	// available.
	Receive() (*Message, error)

	// Close the underlying file descriptor and make subsequent calls
	// to Send and Receive return errors.  Note that transports must
	// allow multiple calls to Close (unlike most io.Closer types,
	// which don't guaratnee this).
	Close()

	// Returns true if the Peer field of Message does not matter
	// because each instance of the transport is connected to a single
	// endpoint (like TCP and unlike an unconnected UDP socket).
	IsConnected() bool

	NewMessage(peer string) *Message
}

var ErrTransportClosed = fmt.Errorf("Transport is closed")

// Implements RFC5531 record-marking protocol for stream sockets
type StreamTransport struct {
	MaxMsgSize int
	Conn       net.Conn

	// Since a StreamTransport is connected, we ignore Peer in sent
	// messages, but can hardcode a value for incoming messages.
	Peer string

	// We jump through a few hoops with atomics because err may be
	// accessed in multiple threads.  We want to make sure all uses of
	// err are synchronized through okay.  Shared read-only access to
	// err is allowed only when okay == 0.  Value -1 is used as an
	// exclusive lock to ensure only one thread updates err.
	okay int32 // 1 = okay, 0 = failed, -1 = failing
	err  error

	mpool sync.Pool
}

// Create a stream transport from a connected stream socket.  This is
// the only valid way to initialize a StreamTransport.  You can
// manually adjust MaxMsgSize after calling this function.
func NewStreamTransport(c net.Conn) *StreamTransport {
	r := &StreamTransport{
		MaxMsgSize: 0x100000,
		Conn:       c,
		okay:       1,
	}
	r.mpool.New = func() any { return &bytes.Buffer{} }
	return r
}

func (tx *StreamTransport) NewMessage(peer string) *Message {
	buf := tx.mpool.Get().(*bytes.Buffer)
	return NewMessage(peer, buf, func() {
		buf.Reset()
		tx.mpool.Put(buf)
	})
}

func (tx *StreamTransport) fail(err error) {
	if atomic.CompareAndSwapInt32(&tx.okay, 1, -1) {
		tx.err = err
		tx.Conn.Close()
		atomic.StoreInt32(&tx.okay, 0)
	}
}

func (tx *StreamTransport) failed() bool {
	for {
		switch atomic.LoadInt32(&tx.okay) {
		case 0:
			return true
		case 1:
			return false
		}
	}
}

// Close the transport and underlying Conn if it hasn't been closed
// yet.  Subsequent calls to Close() have no effect.
func (tx *StreamTransport) Close() {
	tx.fail(ErrTransportClosed)
}

const maxSegment = 0x7fffffff

func (tx *StreamTransport) Send(m *Message) error {
	defer m.Reclaim()
	if tx.failed() {
		return tx.err
	}
	iov := make(net.Buffers, 0, 2)
	for {
		n := m.Len()
		b := make([]byte, 4)
		if n > maxSegment {
			n = maxSegment
			binary.BigEndian.PutUint32(b, uint32(n))
		} else {
			binary.BigEndian.PutUint32(b, 0x80000000|uint32(n))
		}
		iov = append(iov, b)
		iov = append(iov, m.Next(n))
		if m.Len() == 0 {
			break
		}
	}
	if _, err := iov.WriteTo(tx.Conn); err != nil {
		tx.fail(err)
		return err
	}
	return nil
}

func (tx *StreamTransport) Receive() (*Message, error) {
	if tx.failed() {
		return nil, tx.err
	}
	ret := tx.NewMessage(tx.Peer)
	b := make([]byte, 4)
	for b[0]&0x80 == 0 {
		if n, err := tx.Conn.Read(b); n != 4 || err != nil {
			ret.Reclaim()
			tx.fail(err)
			return nil, err
		}
		n := binary.BigEndian.Uint32(b) & 0x7fffffff
		if int(n) > tx.MaxMsgSize-ret.Len() {
			err := fmt.Errorf("Message length %d exceeds maximum %d",
				int(n)+ret.Len(), tx.MaxMsgSize)
			ret.Reclaim()
			tx.fail(err)
			return nil, err
		}
		if _, err := io.CopyN(ret, tx.Conn, int64(n)); err != nil {
			ret.Reclaim()
			tx.fail(err)
			return nil, err
		}
	}
	return ret, nil
}

func (tx *StreamTransport) IsConnected() bool {
	return true
}

var _ Transport = &StreamTransport{}

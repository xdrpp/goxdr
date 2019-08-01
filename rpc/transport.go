
package rpc

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/xdrpp/goxdr/xdr"
	"io"
	"net"
)

type Message struct {
	bytes.Buffer
	Peer string
}

func (m Message) In() xdr.XDR {
	return xdr.XdrIn{&m}
}

func (m Message) Out() xdr.XDR {
	return xdr.XdrOut{&m}
}

type Transport interface {
	// Send a message
	Send(*Message) error

	// Receive the next incoming message
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
			}
			if IsDone(ctx) {
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

// Implements RFC5531 record-marking protocol for stream sockets
type StreamTransport struct {
	MaxMsgSize int
	Conn net.Conn
	Err error
}

func NewStreamTransport(c net.Conn) *StreamTransport {
	return &StreamTransport{
		MaxMsgSize: 0x100000,
		Conn: c,
	}
}

func (tx *StreamTransport) Close() {
	if tx.Conn != nil {
		tx.Conn.Close()
		tx.Conn = nil
	}
}

const maxSegment = 0x7fffffff

func (tx *StreamTransport) Send(m *Message) error {
	if tx.Err != nil {
		return tx.Err
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
	_, tx.Err = iov.WriteTo(tx.Conn)
	if tx.Err != nil {
		tx.Close()
	}
	return tx.Err
}

func (tx *StreamTransport) Receive() (*Message, error) {
	if tx.Err != nil {
		return nil, tx.Err
	}
	var ret Message
	b := make([]byte, 4)
	for b[0]&0x80 == 0 {
		if n, err := tx.Conn.Read(b); n != 4 || err != nil {
			tx.Err = err
			tx.Close()
			return nil, err
		}
		n := binary.BigEndian.Uint32(b) & 0x7fffffff
		if int(n) > tx.MaxMsgSize - ret.Len() {
			tx.Err = fmt.Errorf("Message length %d exceeds maximum %d",
				int(n) + ret.Len(), tx.MaxMsgSize)
			tx.Close()
			return nil, tx.Err
		}
		if _, err := io.CopyN(&ret, tx.Conn, int64(n)); err != nil {
			tx.Err = err
			tx.Close()
			return nil, err
		}
	}
	return &ret, nil
}

func (tx *StreamTransport) IsConnected() bool {
	return true
}

var _ Transport = &StreamTransport{}

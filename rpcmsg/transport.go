
package rpcmsg

import (
	"bytes"
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
	Send(*Message) error
	Receive() (*Message, error)
	Close()
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

var _ Transport = &StreamTransport{}

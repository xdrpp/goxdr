package rpc

import (
	"net"
	"os"
	"time"
)

func ReceiveFile(name string, proc func(*Rpc_msg)) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	conn := NewFileConn(file)
	t := NewStreamTransport(conn)
	for {
		rpcMsg, err := recvRpcMsg(t)
		if err != nil {
			return nil
		}
		proc(rpcMsg)
	}
}

func recvRpcMsg(t Transport) (*Rpc_msg, error) {
	msg, err := t.Receive()
	if err != nil {
		return nil, err
	}
	defer msg.Recycle()
	return GetMsg(msg.In())
}

type fileConn struct {
	*os.File
}

func (f *fileConn) LocalAddr() net.Addr {
	return &net.UnixAddr{Name: f.Name(), Net: "file"}
}

func (f *fileConn) RemoteAddr() net.Addr {
	return &net.UnixAddr{Name: f.Name(), Net: "file"}
}

func (f *fileConn) SetDeadline(t time.Time) error {
	return nil // or return an error if deadlines aren't supported
}

func (f *fileConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (f *fileConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// Usage
func NewFileConn(f *os.File) net.Conn {
	return &fileConn{File: f}
}

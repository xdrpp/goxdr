package rpc_test

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/xdrpp/goxdr/rpc"
)

type Server struct{}

func (*Server) Test_null(ctx context.Context) error                     { return nil }
func (*Server) Test_inc(ctx context.Context, i int32) (int32, error)    { return i + 1, nil }
func (*Server) Test_add(ctx context.Context, i, j int32) (int32, error) { return i + j, nil }
func (*Server) Test_string(ctx context.Context, s string) (string, error) {
	return "Hello " + s, nil
}

var _ TEST_V1 = &Server{}

func streampair() (ret [2]net.Conn) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		panic(err)
	}
	for i := range ret {
		f := os.NewFile(uintptr(fds[i]), "")
		if ret[i], err = net.FileConn(f); err != nil {
			panic(err)
		}
		f.Close()
	}
	return
}

func TestSocketpair(t *testing.T) {
	cs := streampair()
	message := []byte("hello world\n")
	go func() {
		cs[0].Write(message)
		cs[0].Close()
	}()
	if all, err := ioutil.ReadAll(cs[1]); err != nil {
		t.Fatal(err)
	} else if bytes.Compare(all, message) != 0 {
		t.Fatalf("read %q wanted %q", string(all), string(message))
	}
}

func TestChannels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	contents := []string{"one\n", "two\n", "three\n"}
	var ms []*rpc.Message
	msgPool := rpc.NewMsgPool()
	for _, msg := range contents {
		m := msgPool.NewMessage("")
		m.WriteString(msg)
		ms = append(ms, m)
	}

	cs := streampair()
	tx1, tx2 := rpc.NewStreamTransport(cs[0], msgPool), rpc.NewStreamTransport(cs[1], msgPool)
	r := rpc.ReceiveChan(ctx, tx1, 10)
	defer tx1.Close()
	s, _ := rpc.SendChan(tx2, nil, 10)
	go func() {
		defer close(s)
		defer tx2.Close()
		for i := range ms {
			s <- ms[i]
		}
	}()

	for i := 0; ; i++ {
		m := <-r
		if m == nil {
			break
		}
		if m.String() != contents[i] {
			t.Errorf("Received %q wanted %q", m.String(), contents[i])
		}
	}
}

func TestRPC(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cs := streampair()

	mp1 := rpc.NewMsgPool()
	r1 := rpc.NewDriver(ctx, mp1, rpc.NewStreamTransport(cs[0], mp1))
	r1.Register(TEST_V1_Server{&Server{}})
	go func() {
		r1.Go()
		fmt.Println("loop1 returned")
	}()

	mp2 := rpc.NewMsgPool()
	r2 := rpc.NewDriver(ctx, mp2, rpc.NewStreamTransport(cs[1], mp2))
	r2.Log = os.Stderr
	go func() {
		r2.Go()
		fmt.Println("loop2 returned")
	}()

	c := TEST_V1_Client{Send: r2, Ctx: ctx}

	c.Test_null(ctx)
	if r, _ := c.Test_inc(ctx, 1); r != 2 {
		t.Error("Test_inc failed")
	}
	if r, _ := c.Test_add(ctx, 2, 3); r != 5 {
		t.Error("Test_add failed")
	}
	if r, _ := c.Test_string(ctx, "test"); r != "Hello test" {
		t.Error("Test_string failed")
	}
}

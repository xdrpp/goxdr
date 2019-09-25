
package rpc_test

import (
	"bytes"
	"context"
	"fmt"
	"github.com/xdrpp/goxdr/rpc"
	"io/ioutil"
	"net"
	"os"
	"syscall"
	"testing"
)

type Server struct {}
func (*Server) Test_null() {}
func (*Server) Test_inc(i int32) int32 { return i + 1 }
func (*Server) Test_add(i, j int32) int32 { return i + j }
func (*Server) Test_string(s string) string {
	return "Hello " + s
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

	contents := []string{ "one\n", "two\n", "three\n" }
	var ms []*rpc.Message
	for _, msg := range contents {
		m := &rpc.Message{}
		m.WriteString(msg)
		ms = append(ms, m)
	}

	cs := streampair()
	tx1, tx2 := rpc.NewStreamTransport(cs[0]), rpc.NewStreamTransport(cs[1])
	r := rpc.ReceiveChan(ctx, tx1)
	defer tx1.Close()
	s := rpc.SendChan(tx2, nil)
	go func() {
		defer close(s)
		defer tx2.Close()
		for i := range ms {
			s <- ms[i]
		}
	}()

	for i := 0;; i++ {
		m := <- r
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

	r1 := rpc.NewDriver(ctx, rpc.NewStreamTransport(cs[0]))
	r1.Register(TEST_V1_Server{&Server{}})
	go func() {
		r1.Go()
		fmt.Println("loop1 returned")
	}()

	r2 := rpc.NewDriver(ctx, rpc.NewStreamTransport(cs[1]))
	r2.Log = os.Stderr
	go func() {
		r2.Go()
		fmt.Println("loop2 returned")
	}()

	c := TEST_V1_Client { Send: r2, Ctx: ctx }

	c.Test_null()
	if c.Test_inc(1) != 2 {
		t.Error("Test_inc failed")
	}
	if c.Test_add(2, 3) != 5 {
		t.Error("Test_add failed")
	}
	if c.Test_string("test") != "Hello test" {
		t.Error("Test_string failed")
	}
}

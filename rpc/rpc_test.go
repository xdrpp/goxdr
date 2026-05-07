package rpc_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"

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

type blockingServer struct {
	unblock <-chan struct{}
}

func (*blockingServer) Test_null(ctx context.Context) error { return nil }
func (s *blockingServer) Test_inc(ctx context.Context, i int32) (int32, error) {
	select {
	case <-s.unblock:
		return i + 1, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}
func (*blockingServer) Test_add(ctx context.Context, i, j int32) (int32, error) { return i + j, nil }
func (*blockingServer) Test_string(ctx context.Context, str string) (string, error) {
	return "Hello " + str, nil
}

var _ TEST_V1 = &blockingServer{}

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
	if all, err := io.ReadAll(cs[1]); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(all, message) {
		t.Fatalf("read %q wanted %q", string(all), string(message))
	}
}

func TestChannels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	contents := []string{"one\n", "two\n", "three\n"}
	var ms []*rpc.Message
	msgPool := rpc.NewMsgArena(5000)
	for _, msg := range contents {
		m := msgPool.NewMessage("")
		m.WriteString(msg)
		ms = append(ms, m)
	}

	cs := streampair()
	tx1, tx2 := rpc.NewStreamTransportWithPool(cs[0], msgPool, rpc.DefaultMaxMsgSize), rpc.NewStreamTransportWithPool(cs[1], msgPool, rpc.DefaultMaxMsgSize)
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

	c := TEST_V1_Client{Send: r2, Ctx: ctx}

	c.Test_null(ctx)
	if r, err := c.Test_inc(ctx, 1); r != 2 {
		t.Errorf("Test_inc failed, (%v)", err)
	}
	if r, err := c.Test_add(ctx, 2, 3); r != 5 {
		t.Errorf("Test_add failed, (%v)", err)
	}
	if r, err := c.Test_string(ctx, "test"); r != "Hello test" {
		t.Errorf("Test_string failed, (%v)", err)
	}
}

func TestOutgoingCallContextRespectedWithWithoutCancelDriverContext(t *testing.T) {
	parentCtx, parentCancel := context.WithCancel(context.Background())
	clientDriverCtx := context.WithoutCancel(parentCtx)
	parentCancel() // prove driver lifetime does not follow parent cancellation

	cs := streampair()
	unblock := make(chan struct{})
	serverImpl := &blockingServer{unblock: unblock}

	server := rpc.NewDriver(context.Background(), rpc.NewStreamTransport(cs[0]))
	server.Lock = nil // allow multiple blocked calls without serial lock interference
	server.Register(TEST_V1_Server{Srv: serverImpl})
	go server.Go()
	defer server.Close()

	client := rpc.NewDriver(clientDriverCtx, rpc.NewStreamTransport(cs[1]))
	go client.Go()
	defer client.Close()

	c := TEST_V1_Client{Send: client, Ctx: context.Background()}

	t.Run("timeout", func(t *testing.T) {
		callCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		_, err := c.Test_inc(callCtx, 1)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("got %v, want %v", err, context.DeadlineExceeded)
		}
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("call ignored timeout and took too long: %v", elapsed)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		callCtx, cancel := context.WithCancel(context.Background())
		errCh := make(chan error, 1)

		go func() {
			_, err := c.Test_inc(callCtx, 1)
			errCh <- err
		}()

		time.Sleep(20 * time.Millisecond)
		cancel()

		select {
		case err := <-errCh:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want %v", err, context.Canceled)
			}
		case <-time.After(time.Second):
			t.Fatal("call did not return after cancellation")
		}
	})
}

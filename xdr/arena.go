package xdr

import (
	"fmt"
	"os"
	"sync"
	"unsafe"
)

type Arena[T any] struct {
	reset   func(*T)
	objects []T
	//
	lk      sync.Mutex
	free    []*T
	numGet  int
	numMiss int
}

func NewArena[T any](n int, reset func(*T)) *Arena[T] {
	a := &Arena[T]{
		reset:   reset,
		objects: make([]T, n),
		free:    make([]*T, n),
	}
	for i := range a.objects {
		a.free[i] = &a.objects[i]
	}
	return a
}

func (a *Arena[T]) Get() *T {

	a.lk.Lock()
	defer a.lk.Unlock()

	if len(a.free) == 0 {
		fmt.Fprintf(os.Stderr, "xdr rpc arena miss\n")
		a.numMiss += 1
		var obj T
		a.reset(&obj)
		return &obj
	}

	a.numGet += 1

	ptr := a.free[len(a.free)-1]
	a.free = a.free[:len(a.free)-1]

	a.reset(ptr)
	return ptr
}

func (a *Arena[T]) Recycle(x *T) {
	if !contains(a.objects, x) {
		return
	}
	a.reset(x)

	a.lk.Lock()
	defer a.lk.Unlock()
	a.free = append(a.free, x)
}

func (a *Arena[T]) StatString() string {
	a.lk.Lock()
	defer a.lk.Unlock()
	return a.statString()
}

func (a *Arena[T]) statString() string {
	return fmt.Sprintf("num_get=%d num_miss=%d miss_ratio=%.3f%%",
		a.numGet, a.numMiss, 100*float64(a.numMiss)/float64(a.numGet))
}

func contains[T any](slice []T, ptr *T) bool {
	if len(slice) == 0 || ptr == nil {
		return false
	}

	size := unsafe.Sizeof(slice[0])
	first := uintptr(unsafe.Pointer(&slice[0]))
	last := uintptr(unsafe.Pointer(&slice[len(slice)-1]))
	addr := uintptr(unsafe.Pointer(ptr))

	if size == 0 {
		return addr == first
	}
	return addr >= first && addr <= last && (addr-first)%size == 0
}

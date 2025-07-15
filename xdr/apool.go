package xdr

import (
	"fmt"
	"sync"
	"unsafe"
)

type Arena[T any] struct {
	objects []T
	lk      sync.Mutex
	free    []*T
	used    []*T
	numGet  int
	numMiss int
}

func NewArena[T any](n int) *Arena[T] {
	a := &Arena[T]{
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
		a.numMiss += 1
		var z T
		return &z
	}

	a.numGet += 1

	ptr := a.free[len(a.free)-1]
	a.free = a.free[:len(a.free)-1]

	var z T // reset
	*ptr = z
	return ptr
}

func (a *Arena[T]) Recycle(x *T) {
	if !contains(a.objects, x) {
		return
	}

	a.lk.Lock()
	defer a.lk.Unlock()
	a.free = append(a.free, x)
}

func (a *Arena[T]) StatString() string {
	a.lk.Lock()
	defer a.lk.Unlock()
	return fmt.Sprintf("num_get=%d num_miss=%d miss_ratio=%.3f%%",
		a.numGet, a.numMiss, 100*float64(a.numMiss)/float64(a.numGet))
}

func contains[T any](slice []T, ptr *T) bool {
	if len(slice) == 0 || ptr == nil {
		return false
	}

	// Get pointer to first element
	first := unsafe.Pointer(&slice[0])
	// Get pointer to one past the last element
	last := unsafe.Pointer(uintptr(first) + uintptr(len(slice))*unsafe.Sizeof(slice[0]))

	// Check if ptr is within the range [first, last)
	ptrAddr := unsafe.Pointer(ptr)
	return uintptr(ptrAddr) >= uintptr(first) && uintptr(ptrAddr) < uintptr(last)
}

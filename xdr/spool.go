package xdr

import (
	"fmt"
	"sync"
)

type StructPool[T any] struct {
	pool sync.Pool
	// stats
	lk      sync.Mutex
	numGet  int
	numMiss int
}

func NewStructPool[T any]() *StructPool[T] {
	x := &StructPool[T]{}
	x.pool.New = func() any {
		x.lk.Lock()
		x.numMiss += 1
		x.lk.Unlock()
		var z T
		return &z
	}
	return x
}

func (x *StructPool[T]) Get() *T {
	buf := x.pool.Get().(*T)
	var z T
	*buf = z

	x.lk.Lock()
	x.numGet += 1
	x.lk.Unlock()
	return buf
}

func (x *StructPool[T]) Recycle(b *T) {
	x.pool.Put(b)
}

func (x *StructPool[T]) StatString() string {
	x.lk.Lock()
	defer x.lk.Unlock()
	return fmt.Sprintf("num_get=%d num_miss=%d miss_ratio=%.3f%%",
		x.numGet, x.numMiss, 100*float64(x.numMiss)/float64(x.numGet))
}

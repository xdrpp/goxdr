package xdr

import (
	"fmt"
	"sync"
)

type Pool[T any] struct {
	pool  sync.Pool
	reset func(T)
	// stats
	lk      sync.Mutex
	numGet  int
	numMiss int
}

func (x *Pool[T]) SetMkReset(mk func() T, reset func(T)) {
	x.pool.New = func() any {
		x.lk.Lock()
		x.numMiss += 1
		x.lk.Unlock()
		return mk()
	}
	x.reset = reset
}

func (x *Pool[T]) Get() T {
	o := x.pool.Get().(T)
	x.reset(o)

	x.lk.Lock()
	x.numGet += 1
	x.lk.Unlock()
	return o
}

func (x *Pool[T]) Recycle(o T) {
	x.pool.Put(o)
}

func (x *Pool[T]) StatString() string {
	x.lk.Lock()
	defer x.lk.Unlock()
	return fmt.Sprintf("num_get=%d num_miss=%d miss_ratio=%.3f%%",
		x.numGet, x.numMiss, 100*float64(x.numMiss)/float64(x.numGet))
}

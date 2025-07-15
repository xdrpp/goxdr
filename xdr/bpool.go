package xdr

import "sync"

type SmallBytesPool struct {
	size [8]sync.Pool
}

func NewSmallBytesPool() *SmallBytesPool {
	return &SmallBytesPool{
		size: [8]sync.Pool{
			{New: func() any { return []byte{0} }},
			{New: func() any { return []byte{0, 0} }},
			{New: func() any { return []byte{0, 0, 0} }},
			{New: func() any { return []byte{0, 0, 0, 0} }},
			{New: func() any { return []byte{0, 0, 0, 0, 0} }},
			{New: func() any { return []byte{0, 0, 0, 0, 0, 0} }},
			{New: func() any { return []byte{0, 0, 0, 0, 0, 0, 0} }},
			{New: func() any { return []byte{0, 0, 0, 0, 0, 0, 0, 0} }},
		},
	}
}

func (x *SmallBytesPool) Get(size int) []byte {
	// XXX: must zero out
	return x.size[size-1].Get().([]byte)
}

func (x *SmallBytesPool) Recycle(b []byte) {
	x.size[len(b)-1].Put(b)
}

package xdr

import "testing"

func TestArenaRecycleOwnedObject(t *testing.T) {
	arena := NewArena[int](1, func(*int) {})
	obj := arena.Get()
	arena.Recycle(obj)
}

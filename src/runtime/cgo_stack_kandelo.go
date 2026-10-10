//go:build kandelo && cgo

package runtime

import "unsafe"

var cgoCStack [8 << 20]byte

func cgoKandeloThreadInit(threadPointer unsafe.Pointer)

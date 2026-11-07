//go:build kandelo && !cgo

package runtime

import "unsafe"

func cgoKandeloThreadInit(threadPointer unsafe.Pointer) {}

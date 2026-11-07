//go:build kandelo && wasm32

package cgo

import "unsafe"

//go:linkname cgoCallbackKandelo runtime.cgoCallbackKandelo
func cgoCallbackKandelo(fn, frame unsafe.Pointer, ctxt uintptr)

//go:wasmexport crosscall2
//go:nosplit
func crosscall2(fn uintptr, frame unsafe.Pointer, size int32, ctxt uintptr) {
	cgoCallbackKandelo(unsafe.Pointer(fn), frame, ctxt)
}

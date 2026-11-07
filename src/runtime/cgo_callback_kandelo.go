//go:build kandelo && cgo

package runtime

import (
	"internal/abi"
	"unsafe"
)

func cgoCallbackKandeloForeign(fn, frame unsafe.Pointer, ctxt uintptr)

//go:linkname cgoCallbackKandelo
//go:nosplit
func cgoCallbackKandelo(fn, frame unsafe.Pointer, ctxt uintptr) {
	gp := getg()
	if gp != nil && uintptr(fn) == funcHandle(abi.FuncPCABIInternal(kandeloSignalCallback)) {
		// A Go-owned signal is runtime notification, not a user callback.
		// Keep the interrupted goroutine in syscall state: C may still hold
		// pointers into its stack, including the syscall completion frame.
		signum := *(*uint32)(frame)
		systemstack(func() { kandeloGoSignalHandler(signum) })
		return
	}
	if gp == nil {
		needm(false)
		kandeloInitChannelBase()
		getg().m.mOS.foreignCStackActive = 1
		cgoCallbackKandeloForeign(fn, frame, ctxt)
		getg().m.mOS.foreignCStackActive = 0
		dropm()
		return
	}
	if gp != gp.m.curg || gp.m.ncgo == 0 {
		throw("cgo callback without an active Go C call")
	}
	cgocallbackg(fn, frame, ctxt)
}

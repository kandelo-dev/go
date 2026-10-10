//go:build kandelo && cgo

package runtime

import "unsafe"

func cgoCallbackKandeloForeign(fn, frame unsafe.Pointer, ctxt uintptr)

//go:linkname cgoCallbackKandelo
//go:nosplit
func cgoCallbackKandelo(fn, frame unsafe.Pointer, ctxt uintptr) {
	gp := getg()
	if gp == nil {
		needm(false)
		kandeloInitChannelBase()
		cgoCallbackKandeloForeign(fn, frame, ctxt)
		dropm()
		return
	}
	if gp != gp.m.curg || gp.m.ncgo == 0 {
		throw("cgo callback without an active Go C call")
	}
	cgocallbackg(fn, frame, ctxt)
}

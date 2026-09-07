// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import "unsafe"

// wasiPtr is a Go pointer so the GC can track buffers referenced by WASI data.
type wasiPtr = *byte

//go:nosplit
func makeWASIPtr(p *byte) wasiPtr {
	return p
}

func wasiPtrOffset(p wasiPtr, base *byte) uint32 {
	return uint32(uintptr(unsafe.Pointer(p)) - uintptr(unsafe.Pointer(base)))
}

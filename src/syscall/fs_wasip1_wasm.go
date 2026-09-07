// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syscall

import "unsafe"

// wasiPtr represents a 32-bit host pointer on wasm, where Go pointers are 64 bits.
type wasiPtr = uint32

func makeWASIPtr(p *byte) wasiPtr {
	return uint32(uintptr(unsafe.Pointer(p)))
}

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syscall

// wasiPtr is a Go pointer so the GC can track buffers referenced by WASI data.
type wasiPtr = *byte

func makeWASIPtr(p *byte) wasiPtr {
	return p
}

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (wasm || wasm32) && !kandelo

package runtime

import "internal/goarch"

func initWasmMemory() {
	if goarch.IsWasm32 != 0 && firstmoduledata.end > ^uintptr(0)-(physPageSize-1) {
		throw("initial WebAssembly memory exceeds uintptr range")
	}
	initBloc()
	pages := currentMemory()
	if pages < 0 {
		throw("negative WebAssembly memory size")
	}
	if goarch.IsWasm32 != 0 && uintptr(pages) > ^uintptr(0)/physPageSize {
		// Only account for memory whose exclusive end fits in uintptr.
		pages = int32(^uintptr(0) / physPageSize)
	}
	blocMax = uintptr(pages) * physPageSize // record the initial linear memory size
}

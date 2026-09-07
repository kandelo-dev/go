// errorcheck

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasm32

package p

import (
	"structs"
	"unsafe"
)

type pointerHostLayout struct {
	_ structs.HostLayout
	p *int32
	u unsafe.Pointer
}

//go:wasmexport nestedPointers
func nestedPointers(**int32, *pointerHostLayout) {}

type badHostLayout struct {
	_ structs.HostLayout
	s string
}

//go:wasmexport badNestedValue
func badNestedValue(*badHostLayout) {} // ERROR "go:wasmexport: unsupported parameter type"

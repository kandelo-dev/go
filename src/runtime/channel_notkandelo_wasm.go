// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasm && !kandelo

package runtime

// kandeloStartHeapAboveChannel is a no-op on wasm GOOSes other than kandelo
// (js, wasip1): only the Kandelo host reserves a syscall channel region inside
// the initial linear memory. osinit in os_wasm.go calls this unconditionally;
// the kandelo build supplies the real implementation in channel_kandelo.go.
//
//go:nosplit
func kandeloStartHeapAboveChannel() {}

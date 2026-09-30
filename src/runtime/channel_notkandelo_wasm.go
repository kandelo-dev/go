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

// kandeloInitChannelBase is a no-op on wasm GOOSes other than kandelo (js,
// wasip1): they have no syscall channel to capture. osinit in os_wasm.go calls
// this unconditionally; the kandelo build supplies the real implementation in
// channel_kandelo.go.
//
//go:nosplit
func kandeloInitChannelBase() {}

// newosprocKandelo is only reachable from newosproc's GOOS=="kandelo" branch,
// which is dead-code-eliminated on js/wasip1. It must still be defined so the
// package links; it can never be called here.
func newosprocKandelo(mp *m) {
	throw("newosprocKandelo: not supported on this GOOS")
}

// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build (js || wasip1) && (wasm || wasm32)

package runtime

func sigdisable(uint32) {}
func sigenable(uint32)  {}
func sigignore(uint32)  {}
func initsig(bool)      {}

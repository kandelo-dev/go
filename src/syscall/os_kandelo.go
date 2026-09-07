// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package syscall

// ProcExit terminates the process.
//
// STUB: this is the GOOS=kandelo analog of the wasip1 proc_exit host import.
// Kandelo does not yet have a host syscall backend (a later milestone task), so
// this spins instead of exiting. It must not return to callers that treat exit
// as terminal.
func ProcExit(code int32) {
	for {
	}
}

// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build kandelo

package runtime

import "unsafe"

// This file is the GOOS=kandelo analog of os_wasip1.go. It provides the
// runtime OS-layer hooks that the rest of the runtime expects on wasm
// (exit, write1, usleep, readRandom, goenvs, and the clocks).
//
// Every syscall backend here is a STUB for milestone 1: Kandelo does not yet
// have a host syscall channel (that is a later task). The bodies below are
// deliberately non-functional and clearly marked. They exist only so the
// runtime compiles and links for GOOS=kandelo; they do not perform real I/O,
// timekeeping, or randomness. None of them may call throw, because throw's
// own diagnostic path calls write1 and exit.

// exit terminates the process.
//
// STUB: there is no host proc_exit yet. Spin rather than return so callers that
// treat exit as terminal do not fall through. Do not call throw here.
func exit(code int32) {
	for {
	}
}

// write1 writes n bytes from p to the file descriptor fd.
//
// STUB: there is no host write backend yet, so output is discarded. It reports
// n bytes written (never an error) so the runtime's own diagnostic writers do
// not spin retrying. Must not call throw (throw writes via write1).
func write1(fd uintptr, p unsafe.Pointer, n int32) int32 {
	return n
}

// usleep sleeps for usec microseconds.
//
// STUB: no host clock/sleep backend yet; returns immediately.
func usleep(usec uint32) {
}

// readRandom fills r with random bytes.
//
// STUB: no host entropy source yet. Reports zero bytes read so the runtime
// falls back to its weak-seed path rather than assuming entropy it does not
// have.
func readRandom(r []byte) int {
	return 0
}

// goenvs initializes os.Args and the environment.
//
// STUB: no host args/environ backend yet, so both are empty.
func goenvs() {
	argslice = make([]string, 0)
	envs = make([]string, 0)
}

// walltime returns the wall-clock time.
//
// STUB: no host clock backend yet; returns the zero time.
func walltime() (sec int64, nsec int32) {
	return walltime1()
}

func walltime1() (sec int64, nsec int32) {
	return 0, 0
}

// nanotime1 returns a monotonic clock reading in nanoseconds.
//
// STUB: no host clock backend yet; returns 0.
func nanotime1() int64 {
	// STUB: there is no host monotonic clock backend yet. We cannot return a
	// compile-time constant here for two reasons:
	//
	//  1. runtime.main rejects a zero clock reading ("nanotime returning
	//     zero") via throw, which is noreturn. If the compiler can fold this
	//     call to the constant 0, it proves that throw is always taken and
	//     dead-code-eliminates the remainder of runtime.main -- including the
	//     indirect call to main.main. That silently drops the user program's
	//     entry point from the linked module. (os_wasip1.go avoids this only
	//     incidentally, because its nanotime1 calls the clock_time_get
	//     wasmimport, which the compiler cannot fold.)
	//
	//  2. Many parts of the runtime (scheduler, GC, timers) assume a
	//     monotonically non-decreasing clock.
	//
	// Until the host clock backend exists, return a monotonically increasing,
	// always-nonzero counter. This is not a real clock; it only preserves the
	// runtime's startup invariants and keeps nanotime opaque to the optimizer.
	// wasm runs a single M, so a plain increment is race-free here.
	nanotimeCounter += 1000
	return int64(nanotimeCounter)
}

// nanotimeCounter is the backing state for the nanotime1 stub above. It is a
// package-level variable specifically so the compiler cannot constant-fold
// nanotime1's result; see the comment in nanotime1.
var nanotimeCounter uint64 = 1000

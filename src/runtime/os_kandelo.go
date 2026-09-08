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
// The syscall backends route through the Kandelo syscall channel (see
// channel_kandelo.go) except exit, which the kernel exposes as a function
// import. None of these hooks may call throw, because throw's own diagnostic
// path calls write1 and exit.

// kernelExit is the kernel-provided process-exit import. Unlike the other
// syscalls it is a plain wasm function import (the native path the host
// provides), not a channel syscall, so a terminating exit does not need the
// channel handshake to complete.
//
//go:wasmimport kernel kernel_exit
func kernelExit(code int32)

// exit terminates the process via the kernel_exit import. It should not return;
// the loop is defensive so callers that treat exit as terminal do not fall
// through if the import ever does. Do not call throw here.
func exit(code int32) {
	kernelExit(code)
	for {
	}
}

// write1 writes n bytes from p to the file descriptor fd via the channel
// write(2) syscall. Returns the number of bytes written, or -errno on failure.
// Must not call throw (throw writes via write1).
func write1(fd uintptr, p unsafe.Pointer, n int32) int32 {
	ret, errno := doSyscall(sysWrite, int64(fd), int64(uintptr(p)), int64(n))
	if errno != 0 {
		return -errno
	}
	return int32(ret)
}

// usleep sleeps for usec microseconds.
//
// STUB: Kandelo has no channel sleep syscall wired yet; returns immediately.
func usleep(usec uint32) {
}

// readRandom fills r with random bytes via the channel getrandom(2) syscall.
// Returns the number of bytes filled, or 0 on failure so the runtime falls back
// to its weak-seed path rather than assuming entropy it does not have.
func readRandom(r []byte) int {
	if len(r) == 0 {
		return 0
	}
	ret, errno := doSyscall(sysGetrandom, int64(uintptr(unsafe.Pointer(&r[0]))), int64(len(r)), 0)
	if errno != 0 || ret < 0 {
		return 0
	}
	return int(ret)
}

// goenvs initializes os.Args and the environment.
//
// STUB: no host args/environ backend yet, so both are empty.
func goenvs() {
	argslice = make([]string, 0)
	envs = make([]string, 0)
}

// walltime returns the wall-clock time via CLOCK_REALTIME.
func walltime() (sec int64, nsec int32) {
	return walltime1()
}

func walltime1() (sec int64, nsec int32) {
	_, errno := doSyscall(sysClockGettime, kandeloClockRealtime,
		int64(uintptr(unsafe.Pointer(&kandeloTimespec[0]))), 0)
	if errno != 0 {
		return 0, 0
	}
	return kandeloTimespec[0], int32(kandeloTimespec[1])
}

// nanotime1 returns a monotonic clock reading in nanoseconds via
// CLOCK_MONOTONIC.
//
// The clock_gettime call is opaque to the optimizer, so it cannot be
// constant-folded to zero. That matters because runtime.main throws on a zero
// nanotime reading, and a folded zero would let the compiler prove throw is
// always taken and dead-code-eliminate main.main. A real monotonic clock is
// nonzero and non-decreasing, which the runtime scheduler/GC/timers require.
func nanotime1() int64 {
	_, errno := doSyscall(sysClockGettime, kandeloClockMonotonic,
		int64(uintptr(unsafe.Pointer(&kandeloTimespec[0]))), 0)
	if errno != 0 {
		return 0
	}
	return kandeloTimespec[0]*1000000000 + kandeloTimespec[1]
}

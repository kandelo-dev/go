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

// The host exposes process startup metadata (argv and environ) through plain
// kernel.* function imports, not the syscall channel. This mirrors the musl CRT
// contract in host/src/worker-main.ts (buildKernelImports): the guest asks for
// the count, then reads each entry into a guest-owned buffer. Each read is a
// two-step protocol:
//
//   n := kernel_argv_read(i, nil, 0)   // side-effect-free length query
//   kernel_argv_read(i, &buf[0], n)    // exact-capacity copy
//
// The copied bytes are the raw UTF-8 of the entry with NO trailing NUL; the
// returned length is the exact byte count (negative on error). Environ entries
// use the same protocol via kernel_environ_count/kernel_environ_get.

//go:wasmimport kernel kernel_get_argc
func kernel_get_argc() int32

//go:wasmimport kernel kernel_argv_read
func kernel_argv_read(index int32, buf unsafe.Pointer, bufMax int32) int32

//go:wasmimport kernel kernel_environ_count
func kernel_environ_count() int32

//go:wasmimport kernel kernel_environ_get
func kernel_environ_get(index int32, buf unsafe.Pointer, bufMax int32) int32

// readStartupEntry reads argv[index] (arg==true) or environ[index] (arg==false)
// from the host into a freshly allocated string. It returns "" if the host
// reports an error or a non-positive length, which keeps a malformed entry from
// aborting startup.
func readStartupEntry(index int32, arg bool) string {
	read := kernel_environ_get
	if arg {
		read = kernel_argv_read
	}
	n := read(index, nil, 0)
	if n <= 0 {
		return ""
	}
	buf := make([]byte, n)
	got := read(index, unsafe.Pointer(&buf[0]), n)
	if got <= 0 {
		return ""
	}
	return string(buf[:got])
}

// goenvs initializes os.Args and the environment from the host kernel.* startup
// imports, mirroring how os_wasip1.go's goenvs fills argslice and envs from the
// WASI args_get/environ_get calls.
func goenvs() {
	argc := kernel_get_argc()
	if argc < 0 {
		argc = 0
	}
	argslice = make([]string, argc)
	for i := int32(0); i < argc; i++ {
		argslice[i] = readStartupEntry(i, true)
	}

	envc := kernel_environ_count()
	if envc < 0 {
		envc = 0
	}
	envs = make([]string, envc)
	for i := int32(0); i < envc; i++ {
		envs[i] = readStartupEntry(i, false)
	}
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

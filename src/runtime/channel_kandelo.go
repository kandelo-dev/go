// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build kandelo

package runtime

import "unsafe"

// This file implements the Kandelo guest side of the syscall channel: how the
// runtime learns where the channel region lives in linear memory, and the
// atomic handshake that submits a syscall and blocks until the kernel replies.
//
// Channel base discovery (sub-problem B)
// --------------------------------------
// Go's wasm backend cannot import a mutable global, so the guest cannot use the
// C glue's env.__channel_base import. Instead the linker synthesizes an
// exported wasm global named __tls_base (see cmd/link/internal/wasm/asm.go)
// whose constant value is the linear-memory address of kandeloChannelBase
// below. At instantiation the Kandelo host reads that exported global and
// stores the channel-region offset into linear memory at that address (see
// setupChannelBase in host/src/worker-main.ts). We deliberately do NOT export
// __get_channel_base_addr, so the host writes at exactly __tls_base + 0. The
// runtime then reads the offset back from kandeloChannelBase.
//
// kandeloChannelBase is a package-level word so its address is a stable
// link-time constant the linker can embed in the __tls_base global, and so the
// symbol stays live (doSyscall reads it).
var kandeloChannelBase uint32

// Channel region layout, offsets relative to the channel base. Must match
// crates/shared/src/lib.rs (mod channel) in the Kandelo repository.
const (
	chStatus = 0  // u32, atomic
	chNumber = 4  // u32 syscall number
	chArgs   = 8  // 6 * i64 arguments
	chReturn = 56 // i64 return value
	chErrno  = 64 // i32 errno value
	chFlags  = 68 // u32 request flags (always 0 for a plain syscall)
)

// ChannelStatus values (crates/shared/src/lib.rs).
const (
	chIdle    uint32 = 0
	chPending uint32 = 1
	chComplete uint32 = 2
	chError   uint32 = 3
)

// Kandelo syscall numbers (the kernel's own numbering).
const (
	sysWrite        int32 = 4
	sysClockGettime int32 = 40
	sysGetrandom    int32 = 120
)

// Linux-compatible clock ids used by clock_gettime.
const (
	kandeloClockRealtime  int64 = 0
	kandeloClockMonotonic int64 = 1
)

// Atomic helpers implemented in atomic_kandelo.s.
func atomicStore32(addr *uint32, val uint32)
func atomicNotify(addr *uint32, count uint32) uint32
func atomicWait32(addr *uint32, expected uint32, timeout int64) uint32

// doSyscall marshals a syscall into the channel region, runs the atomic
// handshake, and returns the kernel's (return value, errno). It is
// single-threaded: wasm runs a single M and syscalls are synchronous and
// non-reentrant, so a shared channel region and shared scratch buffers are
// safe without locking.
//
// The returned errno is the kernel's errno_value slot; ret is meaningful only
// when errno == 0.
//
//go:nosplit
func doSyscall(number int32, a0, a1, a2 int64) (ret int64, errno int32) {
	base := uintptr(kandeloChannelBase)
	if base == 0 {
		// The host has not provisioned the channel base. Fail honestly rather
		// than scribble on low linear memory; 38 is Linux ENOSYS.
		return 0, 38
	}

	*(*uint32)(unsafe.Pointer(base + chNumber)) = uint32(number)
	*(*int64)(unsafe.Pointer(base + chArgs + 0*8)) = a0
	*(*int64)(unsafe.Pointer(base + chArgs + 1*8)) = a1
	*(*int64)(unsafe.Pointer(base + chArgs + 2*8)) = a2
	*(*int64)(unsafe.Pointer(base + chArgs + 3*8)) = 0
	*(*int64)(unsafe.Pointer(base + chArgs + 4*8)) = 0
	*(*int64)(unsafe.Pointer(base + chArgs + 5*8)) = 0
	*(*uint32)(unsafe.Pointer(base + chFlags)) = 0

	statusPtr := (*uint32)(unsafe.Pointer(base + chStatus))

	// Publish the request: atomic store PENDING, then wake the kernel.
	atomicStore32(statusPtr, chPending)
	atomicNotify(statusPtr, 1)

	// Block until the kernel moves status off PENDING. wait32 returns 1
	// immediately if the value already differs, 0 when notified; a relaxed
	// re-read guards against spurious wakeups (single-threaded guest, so a
	// plain load observes the kernel's completed atomic store).
	for {
		atomicWait32(statusPtr, chPending, -1)
		if *statusPtr != chPending {
			break
		}
	}

	ret = *(*int64)(unsafe.Pointer(base + chReturn))
	errno = *(*int32)(unsafe.Pointer(base + chErrno))

	// Release the channel back to Idle for the next syscall.
	atomicStore32(statusPtr, chIdle)
	return ret, errno
}

// kandeloTimespec mirrors the kernel's struct timespec written by
// clock_gettime: tv_sec then tv_nsec, both 64-bit. It is package-level so its
// address is stable across the synchronous syscall (no stack-copy hazard).
var kandeloTimespec [2]int64

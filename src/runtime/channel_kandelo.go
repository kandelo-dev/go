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
	return doSyscall6(number, a0, a1, a2, 0, 0, 0)
}

// syscall_kandeloSyscall6 is the syscall package's entry into the channel
// handshake. The syscall package declares a bodyless kandeloSyscall6 and this
// linkname supplies the runtime implementation, mirroring how os_wasm.go
// exposes syscall_now as syscall.now. The syscall package needs a low-level
// primitive so os/internal-poll can perform read/write/open/close/etc. against
// the kernel through the same channel the runtime already uses for write1.
//
//go:linkname syscall_kandeloSyscall6 syscall.kandeloSyscall6
//go:nosplit
func syscall_kandeloSyscall6(number int32, a0, a1, a2, a3, a4, a5 int64) (ret int64, errno int32) {
	return doSyscall6(number, a0, a1, a2, a3, a4, a5)
}

//go:nosplit
func doSyscall6(number int32, a0, a1, a2, a3, a4, a5 int64) (ret int64, errno int32) {
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
	*(*int64)(unsafe.Pointer(base + chArgs + 3*8)) = a3
	*(*int64)(unsafe.Pointer(base + chArgs + 4*8)) = a4
	*(*int64)(unsafe.Pointer(base + chArgs + 5*8)) = a5
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

// kandeloStartHeapAboveChannel moves the runtime's break to the top of the
// initial linear memory the Kandelo host committed, so the Go heap grows
// strictly above the per-process syscall channel region.
//
// The problem it solves: Go's wasm allocator (mem_sbrk.go) treats
// [firstmoduledata.end, currentMemory) as heap it may hand out directly, and
// grows past currentMemory with growMemory. The host, however, reserves the
// syscall channel region inside that initial linear memory (just above the
// guest's __heap_base). Without this adjustment the runtime would allocate
// straight across the channel pages and silently corrupt the syscall channel
// once the live heap grew past them.
//
// osinit records currentMemory in blocMax before calling this. Because the host
// guarantees the entire per-process control region -- the channel included --
// lies within that initial linear memory, starting the break at blocMax leaves
// every channel page below the heap. Subsequent heap pages come from sbrk /
// growMemory strictly above it. The only memory forfeited is
// [firstmoduledata.end, channel_top): the module's own ~1 MiB linker init
// headroom plus the channel's few pages, not a large fixed reservation.
//
// The guest deliberately does not model the host's internal control-region
// layout (scratch page, per-thread slots): it only relies on the published
// invariant that everything the host reserved is within the initial memory.
//
// kandeloChannelBase is nonzero only when a Kandelo host provisioned the
// channel; guarding on it keeps this a no-op if the runtime ever links without
// that host contract in place.
//
//go:nosplit
func kandeloStartHeapAboveChannel() {
	if kandeloChannelBase == 0 {
		return
	}
	if bloc < blocMax {
		bloc = blocMax
	}
}

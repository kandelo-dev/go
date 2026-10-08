// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build kandelo

package runtime

import (
	"internal/abi"
	"internal/runtime/atomic"
	"unsafe"
)

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
// symbol stays live. It is the host's write target and a transient handoff
// slot, not the value syscalls read directly: at M init kandeloInitChannelBase
// copies it into the per-M mOS.channelBase, and doSyscall6 reads that per-M
// copy. kandeloStartHeapAboveChannel still reads this word to detect whether a
// Kandelo host provisioned the channel at all.
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
	chIdle     uint32 = 0
	chPending  uint32 = 1
	chComplete uint32 = 2
	chError    uint32 = 3
)

// Kandelo syscall numbers (the kernel's own numbering).
const (
	sysWrite        int32 = 4
	sysClockGettime int32 = 40
	sysGetrandom    int32 = 120
	sysExitGroup    int32 = 387
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

// doSyscall marshals a syscall into this M's channel region, runs the atomic
// handshake, and returns the kernel's (return value, errno).
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

// kandeloInitChannelBase copies this M's syscall-channel base out of the
// transient handoff word (kandeloChannelBase, at the __tls_base address the
// host wrote) and into the M's own mOS.channelBase. Each M is a distinct
// WebAssembly.Instance over the shared linear memory, so kandeloChannelBase is
// a single shared word the host overwrites for each instance just before it
// enters; the M must read it immediately and keep its own copy. getg().m is a
// per-instance value (g is a per-instance wasm global), so this stores into
// the correct M even though every m struct lives in the shared memory.
//
// The main M calls this from osinit. A future thread M will call it from its
// entry trampoline (wasm_pthread_start) before performing any syscall.
// The parent holds kandeloCloneLock until this read and the handoff ack.
//
//go:nosplit
func kandeloInitChannelBase() {
	getg().m.channelBase = uintptr(kandeloChannelBase)
}

// Second-M spawn (kernel_clone) mechanism
// ---------------------------------------
// A Go M on Kandelo is a distinct WebAssembly.Instance sharing the process's
// linear memory. Spawning one is a three-step handoff:
//
//  1. newosprocKandelo (parent M) writes the new M's g0 pointer and g0 stack
//     top into the kandeloThreadHandoff* words, then calls kernel_clone with
//     fnPtr = PC_F of wasmThreadTramp.
//  2. The Kandelo host allocates a thread slot, instantiates a fresh instance
//     over the shared memory, writes that slot's syscall-channel offset to the
//     shared kandeloChannelBase word (host setupChannelBase), and calls
//     table.get(fnPtr)() on the exported __indirect_function_table.
//  3. That call lands in wasmThreadTramp on the new instance, whose g and SP
//     wasm globals are ZERO. The trampoline (asm) installs g and SP from the
//     handoff words, then calls kandeloThreadEntry.
//
// The handoff words are single shared slots. kandeloCloneLock serializes
// concurrent spawns until the child acknowledges that it has consumed g0 and
// stack top and captured its own channel base.

// kandeloThreadHandoffG and kandeloThreadHandoffSP carry the new M's g0 pointer
// and g0 stack top to wasmThreadTramp. They are uint64 so the asm trampoline can
// load them with a single i64 load before any Go stack or g register exists.
var (
	kandeloThreadHandoffG   uint64
	kandeloThreadHandoffSP  uint64
	kandeloThreadHandoffAck uint32
	kandeloCloneLock        mutex
)

// Linux thread-clone flags. The Kandelo kernel's sys_clone requires
// CLONE_VM|CLONE_THREAD to treat the request as a thread rather than a process
// (crates/runtime-core/src/syscalls.rs). The rest mirror musl's pthread_create
// clone mask minus the TID/TLS flags: Go does not use C thread-local storage
// and passes tls/ptid/ctid as 0, so CLONE_SETTLS/CLONE_*_SETTID must stay clear
// to keep the kernel from dereferencing those null pointers.
const (
	_CLONE_VM      = 0x00000100
	_CLONE_FS      = 0x00000200
	_CLONE_FILES   = 0x00000400
	_CLONE_SIGHAND = 0x00000800
	_CLONE_THREAD  = 0x00010000
	_CLONE_SYSVSEM = 0x00040000
)

// kernel_clone spawns a new thread instance. It mirrors the host import used by
// musl's __clone (libc/musl/src/thread/wasm64posix/clone.c) and the Kandelo host
// kernel_clone (host/src/worker-main.ts): the host records fnPtr in the channel
// data area and, after the SYS_CLONE channel transaction returns the new tid,
// instantiates the thread and calls table.get(fnPtr)(). Returns the child tid,
// or a negative errno.
//
//go:wasmimport kernel kernel_clone
func kernel_clone(fnPtr, stackPtr, flags, arg, ptid, tls, ctid uint32) int32

// wasmThreadTramp is the fresh-instance thread entry, implemented in
// atomic_kandelo.s. On entry the g and SP wasm globals are zero; it installs
// them from the handoff words before running any Go code, then calls
// kandeloThreadEntry. newosprocKandelo takes its PC_F via abi.FuncPCABI0.
func wasmThreadTramp()
func kandeloStopWasmLoop()

// kandeloThreadEntry runs on the new M once wasmThreadTramp has installed g and
// SP. It captures its channel, acknowledges the handoff, and enters the Go
// scheduler for an M that owns a P. The no-P path is retained for the explicit
// clone-mechanics probe.
//
//go:nosplit
func kandeloThreadEntry() {
	kandeloInitChannelBase()
	atomic.Store(&kandeloThreadHandoffAck, 1)
	atomicNotify(&kandeloThreadHandoffAck, 1)
	if getg().m.nextp == 0 && getg().m.mstartfn == nil {
		const msg = "M2 alive via kernel_clone\n"
		write1(2, unsafe.Pointer(unsafe.StringData(msg)), int32(len(msg)))
		kandeloStopWasmLoop()
		return
	}
	mstart()
}

// kandeloSpawnProbeM triggers exactly one second-M spawn, independent of the
// scheduler's Gomaxprocs heuristics, so the clone + instance-bootstrap +
// per-M-channel mechanics can be exercised deterministically for this milestone.
// newm allocates a fresh m with its own g0 stack and calls newosproc, which
// clones a new WebAssembly.Instance and runs wasmThreadTramp on it. Exposed via
// linkname so a mechanics-proof test program can request the clone directly.
//
//go:linkname kandeloSpawnProbeM
func kandeloSpawnProbeM() {
	newm(nil, nil, -1)
}

//go:linkname kandeloThreadLockState
func kandeloThreadLockState() bool {
	gp := getg()
	return gp.lockedm.ptr() == gp.m && gp.m.lockedg.ptr() == gp
}

// newosprocKandelo launches a second M via kernel_clone. mp already has an
// allocated g0 with a stack (allocm/newm). May run with m.p==nil; must not use
// write barriers.
//
//go:nowritebarrier
func newosprocKandelo(mp *m) {
	g0 := mp.g0
	// PC_F = symbol value >> 16 = funcValueOffset + defined function index. The
	// host indexes the exported table by this PC_F (writeElementSec fills
	// table[funcValueOffset+i]=func i), so passing it as fnPtr makes
	// table.get(fnPtr)() land in wasmThreadTramp. Never hardcode: the index
	// shifts per build.
	pcF := uint32(abi.FuncPCABI0(wasmThreadTramp) >> 16)
	stackHi := uint64(g0.stack.hi)

	lock(&kandeloCloneLock)
	kandeloThreadHandoffG = uint64(uintptr(unsafe.Pointer(g0)))
	kandeloThreadHandoffSP = stackHi
	atomic.Store(&kandeloThreadHandoffAck, 0)
	flags := uint32(_CLONE_VM | _CLONE_FS | _CLONE_FILES | _CLONE_SIGHAND | _CLONE_THREAD | _CLONE_SYSVSEM)
	ret := kernel_clone(pcF, uint32(stackHi), flags, 0, 0, 0, 0)
	if ret >= 0 {
		for atomic.Load(&kandeloThreadHandoffAck) == 0 {
			if atomicWait32(&kandeloThreadHandoffAck, 0, 15_000_000_000) == 2 {
				unlock(&kandeloCloneLock)
				throw("newosproc: child bootstrap timed out")
			}
		}
	}
	unlock(&kandeloCloneLock)

	if ret < 0 {
		throw("newosproc: kernel_clone failed")
	}
}

//go:nosplit
func doSyscall6(number int32, a0, a1, a2, a3, a4, a5 int64) (ret int64, errno int32) {
	base := getg().m.channelBase
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

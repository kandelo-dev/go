// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Hand-written WebAssembly assembly helpers for the Kandelo syscall-channel
// handshake. They wrap the three WebAssembly threads-proposal atomic memory
// instructions added to the wasm backend (see cmd/internal/obj/wasm). Each
// helper loads its arguments from the Go stack (FP-relative), pushes them onto
// the wasm value stack, emits the atomic op, and stores any result back.

//go:build kandelo && wasm

#include "textflag.h"

// func atomicStore32(addr *uint32, val uint32)
// i32.atomic.store: (i32 addr, i32 value) -> ()
TEXT ·atomicStore32(SB), NOSPLIT, $0-12
	I32Load addr+0(FP) // push address
	I32Load val+8(FP)  // push value
	I32AtomicStore     // memarg align=2 offset=0
	RET

// func atomicNotify(addr *uint32, count uint32) uint32
// memory.atomic.notify: (i32 addr, i32 count) -> i32
TEXT ·atomicNotify(SB), NOSPLIT, $0-20
	Get SP              // result store base
	I32Load addr+0(FP)  // push address
	I32Load count+8(FP) // push count
	MemoryAtomicNotify  // pops (addr,count), pushes woken-count
	I32Store ret+16(FP) // store result
	RET

// func atomicWait32(addr *uint32, expected uint32, timeout int64) uint32
// memory.atomic.wait32: (i32 addr, i32 expected, i64 timeout) -> i32
TEXT ·atomicWait32(SB), NOSPLIT, $0-32
	Get SP                 // result store base
	I32Load addr+0(FP)     // push address
	I32Load expected+8(FP) // push expected
	I64Load timeout+16(FP) // push timeout (i64)
	MemoryAtomicWait32     // pops (addr,expected,timeout), pushes status
	I32Store ret+24(FP)    // store result
	RET

// func wasmThreadTramp()
//
// Fresh-instance thread entry for a second M spawned via kernel_clone. The host
// calls table.get(PC_F)() on a brand-new WebAssembly.Instance, so on entry the
// g and SP wasm globals are ZERO: no Go stack and no current g exist yet. Before
// any Go code runs, install SP and g from the handoff words the parent M wrote
// in newosprocKandelo, mirroring how _rt0_wasm_kandelo bootstraps the main
// instance. Only after that may Go code (kandeloThreadEntry) run.
//
// The single i32 parameter every Go wasm function carries is the resume PC_B;
// the host passes clone's arg (0) into it, i.e. a fresh entry, which we ignore.
TEXT ·wasmThreadTramp(SB), NOSPLIT, $0
	// g = handoff g0 pointer. Load the value via an address constant + indirect
	// load (the idiom runtime·gogo uses), because a direct MOVD sym(SB) value
	// load into a register global is not supported by the wasm assembler.
	MOVD $·kandeloThreadHandoffG(SB), R0
	MOVD 0(R0), R1
	MOVD R1, g

	// SP = handoff g0 stack top. SP is the i32 SP global; wrap the i64 handoff
	// word the same way runtime·mcall installs a stack pointer.
	MOVD $·kandeloThreadHandoffSP(SB), R0
	MOVD 0(R0), R1
	Get R1
	I32WrapI64
	Set SP

	// Enter Go with a fresh PC_B. wasmThreadTramp carries the default Go wasm
	// signature (i32 PC_B) -> (i32 unwind flag): the host calls it as
	// table.get(fnPtr)(0) and reads the i32 result as the thread exit status.
	// kandeloThreadEntry does not switch goroutines, so it returns 0 (no
	// unwind); leave that i32 on the stack as this function's result rather than
	// dropping it, or the module fails validation (result [i32] but got []).
	I32Const $0
	Call ·kandeloThreadEntry(SB)
	Return

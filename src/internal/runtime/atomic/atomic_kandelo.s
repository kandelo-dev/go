// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Real WebAssembly atomic operations for the kandelo port (shared memory).
// Each helper loads its arguments from the Go stack (FP-relative), pushes them
// onto the wasm value stack, emits a single WebAssembly threads-proposal atomic
// instruction, and stores any result back to the Go stack.
//
// Value-returning helpers push SP first to provide the base address for the
// final I32Store/I64Store of the result. The rmw ops leave the PRIOR value on
// the stack; the cmpxchg ops leave the loaded value; and8/or8 discard the
// prior value with Drop.

//go:build kandelo && wasm

#include "textflag.h"

// --- loads ---

// func atomicLoad32(ptr *uint32) uint32
TEXT ·atomicLoad32(SB), NOSPLIT, $0-12
	Get SP
	I32Load ptr+0(FP)
	I32AtomicLoad
	I32Store ret+8(FP)
	RET

// func atomicLoad64(ptr *uint64) uint64
TEXT ·atomicLoad64(SB), NOSPLIT, $0-16
	Get SP
	I32Load ptr+0(FP)
	I64AtomicLoad
	I64Store ret+8(FP)
	RET

// func atomicLoad8(ptr *uint8) uint32
TEXT ·atomicLoad8(SB), NOSPLIT, $0-12
	Get SP
	I32Load ptr+0(FP)
	I32AtomicLoad8U
	I32Store ret+8(FP)
	RET

// --- stores ---

// func atomicStore32(ptr *uint32, val uint32)
TEXT ·atomicStore32(SB), NOSPLIT, $0-12
	I32Load ptr+0(FP)
	I32Load val+8(FP)
	I32AtomicStore
	RET

// func atomicStore64(ptr *uint64, val uint64)
TEXT ·atomicStore64(SB), NOSPLIT, $0-16
	I32Load ptr+0(FP)
	I64Load val+8(FP)
	I64AtomicStore
	RET

// func atomicStore8(ptr *uint8, val uint32)
TEXT ·atomicStore8(SB), NOSPLIT, $0-12
	I32Load ptr+0(FP)
	I32Load val+8(FP)
	I32AtomicStore8
	RET

// --- rmw add ---

// func atomicAdd32(ptr *uint32, delta uint32) uint32
TEXT ·atomicAdd32(SB), NOSPLIT, $0-20
	Get SP
	I32Load ptr+0(FP)
	I32Load delta+8(FP)
	I32AtomicRmwAdd
	I32Store ret+16(FP)
	RET

// func atomicAdd64(ptr *uint64, delta uint64) uint64
TEXT ·atomicAdd64(SB), NOSPLIT, $0-24
	Get SP
	I32Load ptr+0(FP)
	I64Load delta+8(FP)
	I64AtomicRmwAdd
	I64Store ret+16(FP)
	RET

// --- rmw and/or (32-bit, return prior) ---

// func atomicAnd32(ptr *uint32, val uint32) uint32
TEXT ·atomicAnd32(SB), NOSPLIT, $0-20
	Get SP
	I32Load ptr+0(FP)
	I32Load val+8(FP)
	I32AtomicRmwAnd
	I32Store ret+16(FP)
	RET

// func atomicOr32(ptr *uint32, val uint32) uint32
TEXT ·atomicOr32(SB), NOSPLIT, $0-20
	Get SP
	I32Load ptr+0(FP)
	I32Load val+8(FP)
	I32AtomicRmwOr
	I32Store ret+16(FP)
	RET

// --- rmw and/or (8-bit, discard prior) ---

// func atomicAnd8(ptr *uint8, val uint32)
TEXT ·atomicAnd8(SB), NOSPLIT, $0-12
	I32Load ptr+0(FP)
	I32Load val+8(FP)
	I32AtomicRmw8AndU
	Drop
	RET

// func atomicOr8(ptr *uint8, val uint32)
TEXT ·atomicOr8(SB), NOSPLIT, $0-12
	I32Load ptr+0(FP)
	I32Load val+8(FP)
	I32AtomicRmw8OrU
	Drop
	RET

// --- rmw xchg ---

// func atomicXchg32(ptr *uint32, new uint32) uint32
TEXT ·atomicXchg32(SB), NOSPLIT, $0-20
	Get SP
	I32Load ptr+0(FP)
	I32Load new+8(FP)
	I32AtomicRmwXchg
	I32Store ret+16(FP)
	RET

// func atomicXchg64(ptr *uint64, new uint64) uint64
TEXT ·atomicXchg64(SB), NOSPLIT, $0-24
	Get SP
	I32Load ptr+0(FP)
	I64Load new+8(FP)
	I64AtomicRmwXchg
	I64Store ret+16(FP)
	RET

// --- rmw cmpxchg (return loaded value) ---

// func atomicCas32(ptr *uint32, old, new uint32) uint32
TEXT ·atomicCas32(SB), NOSPLIT, $0-20
	Get SP
	I32Load ptr+0(FP)
	I32Load old+8(FP)
	I32Load new+12(FP)
	I32AtomicRmwCmpxchg
	I32Store ret+16(FP)
	RET

// func atomicCas64(ptr *uint64, old, new uint64) uint64
TEXT ·atomicCas64(SB), NOSPLIT, $0-32
	Get SP
	I32Load ptr+0(FP)
	I64Load old+8(FP)
	I64Load new+16(FP)
	I64AtomicRmwCmpxchg
	I64Store ret+24(FP)
	RET

// --- fence ---

// func atomicFence()
TEXT ·atomicFence(SB), NOSPLIT, $0-0
	AtomicFence
	RET

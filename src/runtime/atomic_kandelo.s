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

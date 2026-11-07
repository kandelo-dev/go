// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build kandelo && cgo

#include "go_asm.h"
#include "textflag.h"

// func asmcgocall(fn, arg unsafe.Pointer) int32
TEXT ·asmcgocall(SB), NOSPLIT, $0-12
	MOVW g_m(g), R2
	MOVD $runtime·m0(SB), R3
	Get R2
	Get R3
	I64Eq
	If
		MOVD $runtime·cgoCStack+(8<<20)(SB), R1
	Else
		MOVW m_cgoThread(R2), R1
	End
	Get R1
	I64Eqz
	If
		MOVW (m_mOS+mOS_foreignCStackActive)(R2), R1
		Get R1
		I64Eqz
		If
			UNDEF
		End
	End
	// A function table entry has a fixed Wasm type. cgo's errno wrappers
	// return i32, ordinary wrappers and runtime hooks return void.
	I32Const $runtime·wasmCgoCallSignatures(SB)
	I32Load fn+0(FP)
	I32Add
	I32Load8U $0
	I64ExtendI32U
	Set R0
	Get R0
	I64Const $2
	I64Eq
	If
		Get SP
		I32Load arg+4(FP)
		I32Load fn+0(FP)
		CallIndirect $0
		I32Store ret+8(FP)
	Else
		Get R0
		I64Const $1
		I64Ne
		If
			UNDEF // not a C argument-frame function
		End
		I32Load arg+4(FP)
		I32Load fn+0(FP)
		CallIndirect $1
		MOVW $0, ret+8(FP)
	End
	RET

TEXT _cgo_topofstack(SB), NOSPLIT|NOFRAME, $0
	MOVW g_m(g), R1
	MOVW m_curg(R1), R2
	MOVW g_stack+stack_hi(R2), R3
	Get R3
	I32WrapI64
	Return

TEXT runtime·setg(SB), NOSPLIT, $0-4
	MOVW gg+0(FP), R0
	MOVD R0, g
	RET

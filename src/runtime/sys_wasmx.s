// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build wasm || wasm32

#include "textflag.h"

TEXT runtime·wasmDiv(SB), NOSPLIT, $0-0
	Get R0
	I64Const $-0x8000000000000000
	I64Eq
	If
		Get R1
		I64Const $-1
		I64Eq
		If
			I64Const $-0x8000000000000000
			Return
		End
	End
	Get R0
	Get R1
	I64DivS
	Return

#ifdef GOOS_kandelo
// kernelExit's Wasm import wrapper reads its status at SP+8. Switch to a
// permanent zero frame before publishing that the retiring stack is free.
GLOBL runtime·kandeloThreadExitFrame(SB), RODATA|NOPTR, $16

TEXT runtime·exitThread(SB), NOSPLIT|NOFRAME, $0-4
	I32Load wait+0(FP)
	MOVD $runtime·kandeloThreadExitFrame(SB), R0
	Get R0
	I32WrapI64
	Set SP
	I32Const $0
	I32AtomicStore
	I32Const $0
	Call runtime·kernelExit(SB)
	UNDEF
#else
TEXT runtime·exitThread(SB), NOSPLIT, $0-0
	UNDEF
#endif

TEXT runtime·osyield(SB), NOSPLIT, $0-0
#ifdef GOOS_kandelo
	RET
#else
	UNDEF
#endif

TEXT runtime·currentMemory(SB), NOSPLIT, $0
	Get SP
	CurrentMemory
	I32Store ret+0(FP)
	RET

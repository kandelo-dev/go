// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "textflag.h"

TEXT ·set_crosscall2(SB),NOSPLIT,$0-0
#ifdef GOOS_kandelo
	CALL runtime·kandeloCgoPrepareEnv(SB)
	I64Const $runtime·kandeloCgoStartupArgs(SB)
	I32WrapI64
#else
	I32Const $0
#endif
	I64Const $x_cgo_kandelo_init(SB)
	I64Const $16
	I64ShrU
	I32WrapI64
	CallIndirect $1
	RET

#ifndef GOOS_kandelo
TEXT crosscall2(SB), NOSPLIT, $0
	UNDEF
#endif

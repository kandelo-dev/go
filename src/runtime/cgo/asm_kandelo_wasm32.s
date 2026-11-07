//go:build kandelo

// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "textflag.h"

TEXT ·set_crosscall2(SB),NOSPLIT,$0-0
	CALL runtime·kandeloCgoPrepareEnv(SB)
	I32Const $runtime·kandeloCgoStartupArgs(SB)
	I32Const $x_cgo_kandelo_init(SB)
	CallIndirect $1
	RET

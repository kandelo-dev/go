//go:build kandelo && cgo

#include "textflag.h"

TEXT ·cgoKandeloThreadInit(SB), NOSPLIT, $0-8
	I64Load threadPointer+0(FP)
	I32WrapI64
	I64Const $x_cgo_kandelo_thread_init(SB)
	I64Const $16
	I64ShrU
	I32WrapI64
	CallIndirect $1
	RET

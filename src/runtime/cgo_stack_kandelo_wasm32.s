//go:build kandelo && cgo && wasm32

#include "textflag.h"

TEXT ·cgoKandeloThreadInit(SB), NOSPLIT, $0-4
	I32Load threadPointer+0(FP)
	I32Const $x_cgo_kandelo_thread_init(SB)
	CallIndirect $1
	RET

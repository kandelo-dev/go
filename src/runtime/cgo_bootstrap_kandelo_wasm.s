//go:build kandelo && cgo

#include "textflag.h"
#include "go_asm.h"

TEXT wasm_export_cgo_thread_bootstrap(SB), NOSPLIT|NOFRAME, $0
	Get R0
	I64ExtendI32U
	I64Const $1
	I64Add
	I64Const $32768
	I64Mul
	Set R2
	MOVD $runtime·kandeloCgoBootstrapStackBase(SB), R1
	MOVD 0(R1), R3
	Get R3
	Get R2
	I64Add
	I64Const $16
	I64Sub
	I32WrapI64
	Set SP
	Return

TEXT runtime·kandeloCgoReadChannelBase(SB), NOSPLIT, $0-8
	I64Load base+0(FP)
	I32WrapI64
	I64Const $x_cgo_kandelo_get_channel_base(SB)
	I64Const $16
	I64ShrU
	I32WrapI64
	CallIndirect $1
	RET

TEXT runtime·cgoCallbackKandeloForeign(SB), NOSPLIT|TOPFRAME, $0-24
	MOVD fn+0(FP), R0
	MOVD frame+8(FP), R1
	MOVD ctxt+16(FP), R2
	MOVD g_m(g), R3
	MOVD m_curg(R3), R4
	MOVD m_g0(R3), R5
	MOVD (g_stack+stack_hi)(R4), R6
	Get R6
	I64Load (g_sched+gobuf_sp)(R4)
	I64Sub
	Set R6
	MOVD R6, (m_mOS+mOS_callbackCurgStackOffset)(R3)
	MOVD (g_sched+gobuf_pc)(R4), R6
	MOVD R6, (m_mOS+mOS_callbackCurgPC)(R3)
	MOVD (g_sched+gobuf_sp)(R5), R6
	MOVD R6, (m_mOS+mOS_callbackG0SP)(R3)
	MOVD SP, (g_sched+gobuf_sp)(R5)
	MOVD (g_sched+gobuf_sp)(R4), R7
	MOVD R4, g
	MOVD R7, SP
	MOVD R0, 0(SP)
	MOVD R1, 8(SP)
	MOVD R2, 16(SP)
	CALL runtime·cgocallbackg(SB)
	MOVD g_m(g), R3
	MOVD (g_stack+stack_hi)(g), R6
	Get R6
	I64Load (m_mOS+mOS_callbackCurgStackOffset)(R3)
	I64Sub
	Set R6
	MOVD R6, (g_sched+gobuf_sp)(g)
	MOVD (m_mOS+mOS_callbackCurgPC)(R3), R6
	MOVD R6, (g_sched+gobuf_pc)(g)
	MOVD m_g0(R3), R5
	MOVD R5, g
	MOVD (g_sched+gobuf_sp)(R5), SP
	MOVD (m_mOS+mOS_callbackG0SP)(R3), R6
	MOVD R6, (g_sched+gobuf_sp)(R5)
	RET

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
	MOVW $runtime·kandeloCgoBootstrapStackBase(SB), R1
	MOVW 0(R1), R3
	Get R3
	Get R2
	I64Add
	I64Const $16
	I64Sub
	I32WrapI64
	Set SP
	Return

TEXT runtime·kandeloCgoReadChannelBase(SB), NOSPLIT, $0-4
	I32Load base+0(FP)
	I32Const $x_cgo_kandelo_get_channel_base(SB)
	CallIndirect $1
	RET

TEXT runtime·cgoCallbackKandeloForeign(SB), NOSPLIT|TOPFRAME, $0-12
	MOVW fn+0(FP), R0
	MOVW frame+4(FP), R1
	MOVW ctxt+8(FP), R2
	MOVW g_m(g), R3
	MOVW m_curg(R3), R4
	MOVW m_g0(R3), R5
	MOVW (g_stack+stack_hi)(R4), R6
	Get R6
	I64Load32U (g_sched+gobuf_sp)(R4)
	I64Sub
	Set R6
	MOVW R6, (m_mOS+mOS_callbackCurgStackOffset)(R3)
	MOVW (g_sched+gobuf_pc)(R4), R6
	MOVW R6, (m_mOS+mOS_callbackCurgPC)(R3)
	MOVW (g_sched+gobuf_lr)(R4), R6
	MOVW R6, (m_mOS+mOS_callbackCurgHandle)(R3)
	MOVW (g_sched+gobuf_sp)(R5), R6
	MOVW R6, (m_mOS+mOS_callbackG0SP)(R3)
	MOVW SP, (g_sched+gobuf_sp)(R5)
	MOVW (g_sched+gobuf_sp)(R4), R7
	MOVD R4, g
	MOVW R7, SP
	MOVW R0, 0(SP)
	MOVW R1, 4(SP)
	MOVW R2, 8(SP)
	CALL runtime·cgocallbackg(SB)
	MOVW g_m(g), R3
	MOVW (g_stack+stack_hi)(g), R6
	Get R6
	I64Load32U (m_mOS+mOS_callbackCurgStackOffset)(R3)
	I64Sub
	Set R6
	MOVW R6, (g_sched+gobuf_sp)(g)
	MOVW (m_mOS+mOS_callbackCurgPC)(R3), R6
	MOVW R6, (g_sched+gobuf_pc)(g)
	MOVW (m_mOS+mOS_callbackCurgHandle)(R3), R6
	MOVW R6, (g_sched+gobuf_lr)(g)
	MOVW m_g0(R3), R5
	MOVD R5, g
	MOVW (g_sched+gobuf_sp)(R5), SP
	MOVW (m_mOS+mOS_callbackG0SP)(R3), R6
	MOVW R6, (g_sched+gobuf_sp)(R5)
	RET

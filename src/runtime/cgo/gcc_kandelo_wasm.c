//go:build kandelo && wasm

#include <stddef.h>

extern unsigned long __wasm_tp_storage[64];
extern _Thread_local unsigned long __wasm_thread_pointer;
extern int __init_tp(void *thread_pointer);
extern void (*x_crosscall2_ptr)(void (*fn)(void *), void *, int, size_t);

void crosscall2(void (*fn)(void *), void *arg, int size, size_t context)
{
	__builtin_trap();
}

void x_cgo_kandelo_init(void *arg)
{
	__wasm_thread_pointer = (unsigned long)__wasm_tp_storage;
	if (__init_tp(__wasm_tp_storage) < 0)
		__builtin_trap();
	x_crosscall2_ptr = crosscall2;
}

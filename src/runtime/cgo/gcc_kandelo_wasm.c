//go:build kandelo && wasm

#include <stddef.h>
#include <stdint.h>

extern unsigned long __wasm_tp_storage[64];
extern _Thread_local unsigned long __wasm_thread_pointer;
extern int __init_tp(void *thread_pointer);
extern void (*x_crosscall2_ptr)(void (*fn)(void *), void *, int, size_t);

extern void crosscall2(void (*fn)(void *), void *arg, int size, size_t context);
extern void __init_libc(char **envp, char *program_name);
extern uint32_t __kandelo_cgo_ctor_count;
extern void (*__kandelo_cgo_ctors[])(void);

struct kandelo_cgo_startup_args {
	uint32_t envp;
	uint32_t program_name;
};

void x_cgo_kandelo_init(void *arg)
{
	struct kandelo_cgo_startup_args *startup = arg;
	if (!startup)
		__builtin_trap();
	__init_libc((char **)(uintptr_t)startup->envp,
		(char *)(uintptr_t)startup->program_name);
	x_crosscall2_ptr = crosscall2;
	for (uint32_t index = 0; index < __kandelo_cgo_ctor_count; ++index)
		__kandelo_cgo_ctors[index]();
}

void x_cgo_kandelo_thread_init(void *thread_pointer)
{
	if (__init_tp(thread_pointer) < 0)
		__builtin_trap();
}

//go:build kandelo && wasm

#include <stddef.h>
#include <stdint.h>
#include <kandelo/signal.h>
#include <kandelo/syscall.h>

extern unsigned long __wasm_tp_storage[64];
extern _Thread_local unsigned long __wasm_thread_pointer;
extern int __init_tp(void *thread_pointer);
extern void (*x_crosscall2_ptr)(void (*fn)(void *), void *, int, size_t);

extern void crosscall2(void (*fn)(void *), void *arg, int size, size_t context);
extern void __init_libc(char **envp, char *program_name);
extern uint32_t __kandelo_cgo_ctor_count;
extern void (*__kandelo_cgo_ctors[])(void);

__asm__(".globaltype __channel_base, i32\n");

__attribute__((noinline)) void x_cgo_kandelo_get_channel_base(uint32_t *base)
{
	uint32_t current;
	__asm__ volatile("global.get __channel_base\n"
	                 "local.set %0" : "=r"(current));
	*base = current;
}

__attribute__((noinline)) void __wasm_thread_init(unsigned long thread_pointer)
{
	__wasm_thread_pointer = thread_pointer;
}

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
	__wasm_thread_init((unsigned long)thread_pointer);
	if (__init_tp(thread_pointer) < 0)
		__builtin_trap();
}

static uint32_t signal_callback;
static _Thread_local uint64_t *signal_notifications;

static void kandelo_signal_handler(int signal)
{
	uint32_t argument = (uint32_t)signal;
	/* A Go syscall has already released its P and is not a Go-to-C call.
	 * Queue its notifications for Go to send on the system stack. A real
	 * C call or a foreign pthread uses the normal callback transition. */
	if (signal_notifications) {
		*signal_notifications |= UINT64_C(1) << (signal - 1);
		return;
	}
	crosscall2((void (*)(void *))(uintptr_t)signal_callback, &argument, sizeof(argument), 0);
}

void x_cgo_kandelo_signal_setup(void *argument)
{
	uint32_t *setup = argument;
	signal_callback = setup[0];
	setup[1] = (uint32_t)(uintptr_t)kandelo_signal_handler;
}

void x_cgo_kandelo_signal_dispatch(void *argument)
{
	uint64_t notifications = 0;
	uint64_t *previous = signal_notifications;
	signal_notifications = &notifications;
	__wasm_posix_dispatch_pending_signal();
	signal_notifications = previous;
	*(uint64_t *)argument = notifications;
}

struct kandelo_go_syscall {
	int64_t arguments[7];
	struct kandelo_syscall_result result;
};

void x_cgo_kandelo_syscall(void *argument)
{
	struct kandelo_go_syscall *call = argument;
	__wasm_posix_syscall6((int32_t)call->arguments[0], call->arguments + 1, &call->result);
}

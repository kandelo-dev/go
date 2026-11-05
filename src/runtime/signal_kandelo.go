// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build kandelo

package runtime

// Kandelo delivers asynchronous signals at syscall boundaries. This file
// queues Go notifications there; cgo calls use libc's normal C dispatcher.
// It does not provide native signal-context unwinding, asynchronous
// preemption, or a signal-mask management thread.

import (
	"internal/abi"
	"unsafe"
)

const (
	kandeloSigHandler = 65556
	kandeloSigOldMask = 65572
)

type kandeloSigaction struct {
	handler uint32
	flags   uint32
	mask    uint64
}

var kandeloSignalHandler uint32
var kandeloPreviousSignals [65]kandeloSigaction
var kandeloSavedSignals [65]bool

//go:linkname _cgo_kandelo_signal_setup _cgo_kandelo_signal_setup
var _cgo_kandelo_signal_setup unsafe.Pointer

//go:linkname _cgo_kandelo_signal_dispatch _cgo_kandelo_signal_dispatch
var _cgo_kandelo_signal_dispatch unsafe.Pointer

func kandeloGoSignalHandler(signum uint32) {
	if sigsend(signum) || signal_ignored(signum) {
		return
	}
	// These asynchronous signals terminate an ordinary Go program when
	// no notification channel accepts them. A delivery can already be in
	// flight when Stop removes its last channel, so restore the default
	// and raise it again rather than silently dropping that delivery.
	if signum == 1 || signum == 2 || signum == 15 {
		action := kandeloSigaction{}
		mask := uint64(1) << (signum - 1)
		doSyscall6Raw(sysSigaction, int64(signum), int64(uintptr(unsafe.Pointer(&action))), 0, 0, 0, 0)
		doSyscall6Raw(37, 1, int64(uintptr(unsafe.Pointer(&mask))), 0, 8, 0, 0)
		doSyscall6Raw(sysRaise, int64(signum), 0, 0, 0, 0, 0)
		KeepAlive(&action)
		KeepAlive(&mask)
		throw("Kandelo fatal signal did not terminate")
	}
}

// Kandelo has Linux-compatible signal numbers. As on other Go Unix
// targets, nonfatal asynchronous signals are caught without notifying
// user code until Notify is called. Leave job-control and fatal signals
// to the kernel, and leave musl's private signals 32-34 alone.
func kandeloDefaultGoSignal(signum uint32) bool {
	switch signum {
	case 10, 12, 13, 14, 17, 23, 24, 25, 26, 27, 28, 29, 30:
		return true
	}
	return signum >= 35 && signum <= 64
}

func initsig(preinit bool) {
	if preinit {
		return
	}
	for signum := uint32(1); signum <= 64; signum++ {
		var action kandeloSigaction
		_, errno := doSyscall6(sysSigaction, int64(signum), 0, int64(uintptr(unsafe.Pointer(&action))), 0, 0, 0)
		if errno != 0 {
			throw("cannot read Kandelo signal action")
		}
		if action.handler == 1 {
			// SIGPIPE is ignored by the target's early I/O setup, not by
			// the embedding parent. os.sigpipe owns stdout/stderr policy.
			if signum != 13 {
				sigInitIgnored(signum)
			}
		} else if action.handler == 0 && kandeloDefaultGoSignal(signum) {
			sigenable(signum)
			kandeloPreviousSignals[signum] = kandeloSigaction{handler: kandeloSignalHandler}
		}
		KeepAlive(&action)
	}
}

func kandeloSignalCallback(frame unsafe.Pointer) {
	kandeloGoSignalHandler(*(*uint32)(frame))
}

func sigenable(signum uint32) {
	if signum == 0 || signum >= 65 || signum == 9 || signum == 19 || (signum >= 32 && signum <= 34) {
		return
	}
	if kandeloSignalHandler == 0 {
		if iscgo {
			if _cgo_kandelo_signal_setup == nil {
				throw("missing Kandelo cgo signal setup")
			}
			args := [2]uint32{uint32(abi.FuncPCABIInternal(kandeloSignalCallback) >> 16), 0}
			asmcgocall(_cgo_kandelo_signal_setup, unsafe.Pointer(&args[0]))
			kandeloSignalHandler = args[1]
		} else {
			kandeloSignalHandler = uint32(abi.FuncPCABIInternal(kandeloGoSignalHandler) >> 16)
		}
	}
	kandeloSetSignal(signum, kandeloSigaction{handler: kandeloSignalHandler})
}

func sigignore(signum uint32) {
	if signum == 0 || signum >= 65 || signum == 9 || signum == 19 || (signum >= 32 && signum <= 34) {
		return
	}
	kandeloSetSignal(signum, kandeloSigaction{handler: 1})
}

func sigdisable(signum uint32) {
	if signum == 0 || signum >= 65 || signum == 9 || signum == 19 || !kandeloSavedSignals[signum] {
		return
	}
	action := kandeloPreviousSignals[signum]
	_, errno := doSyscall6(sysSigaction, int64(signum), int64(uintptr(unsafe.Pointer(&action))), 0, 0, 0, 0)
	if errno != 0 {
		throw("cannot restore Kandelo signal action")
	}
	KeepAlive(&action)
}

func kandeloSetSignal(signum uint32, action kandeloSigaction) {
	var previous kandeloSigaction
	_, errno := doSyscall6(sysSigaction, int64(signum), int64(uintptr(unsafe.Pointer(&action))), int64(uintptr(unsafe.Pointer(&previous))), 0, 0, 0)
	if errno != 0 {
		throw("cannot install Kandelo signal action")
	}
	if !kandeloSavedSignals[signum] {
		kandeloPreviousSignals[signum] = previous
		kandeloSavedSignals[signum] = true
	}
	KeepAlive(&action)
	KeepAlive(&previous)
}

//go:nosplit
func kandeloDeliverSignals() {
	mp := getg().m
	base := mp.channelBase
	if base == 0 || mp.signalDeliveryActive || *(*uint32)(unsafe.Pointer(base + chSigSignum)) == 0 {
		return
	}
	mp.signalDeliveryActive = true
	for {
		signum := *(*uint32)(unsafe.Pointer(base + chSigSignum))
		if signum == 0 {
			break
		}
		if iscgo {
			var notifications uint64
			asmcgocall(_cgo_kandelo_signal_dispatch, unsafe.Pointer(&notifications))
			systemstack(func() {
				for signal := uint32(1); signal <= 64; signal++ {
					if notifications&(uint64(1)<<(signal-1)) != 0 {
						kandeloGoSignalHandler(signal)
					}
				}
			})
			KeepAlive(&notifications)
			continue
		}
		handler := *(*uint32)(unsafe.Pointer(base + kandeloSigHandler))
		// The mask is an unaligned 64-bit wire value, shared with wasm32 C.
		oldMask := uint64(*(*uint32)(unsafe.Pointer(base + kandeloSigOldMask))) |
			uint64(*(*uint32)(unsafe.Pointer(base + kandeloSigOldMask + 4)))<<32
		*(*uint32)(unsafe.Pointer(base + chSigSignum)) = 0
		if handler != kandeloSignalHandler || handler == 0 {
			throw("unknown Kandelo Go signal handler")
		}
		// A syscall can return on g0 or while the user goroutine has no P.
		// Queue the notification on the system stack rather than growing a
		// goroutine whose state is still Gsyscall.
		systemstack(func() { kandeloGoSignalHandler(signum) })
		_, errno := doSyscall6Raw(208, 0, 0, 0, 0, 0, 0)
		if errno != 0 {
			throw("cannot finish Kandelo signal delivery")
		}
		_, errno = doSyscall6Raw(37, 2, int64(uintptr(unsafe.Pointer(&oldMask))), 0, 8, 0, 0)
		if errno != 0 {
			throw("cannot restore Kandelo signal mask")
		}
		KeepAlive(&oldMask)
	}
	mp.signalDeliveryActive = false
}

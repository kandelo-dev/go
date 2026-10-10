// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build kandelo

package runtime

import "unsafe"

const isSbrkPlatform = false

const (
	kandeloMapPrivate = 0x02
	kandeloMapFixed   = 0x10
	kandeloMapAnon    = 0x20
)

func initWasmMemory() {}

func sysReserveAlignedSbrk(size, align uintptr) (unsafe.Pointer, uintptr) {
	throw("sbrk allocator is not supported on Kandelo")
	return nil, 0
}

//go:nosplit
func kandeloMmap(address unsafe.Pointer, size uintptr, protection, flags int32) unsafe.Pointer {
	result, errno := doSyscall6(46, int64(uintptr(address)), int64(size), int64(protection), int64(flags), -1, 0)
	if errno != 0 || result <= 0 || result == 0xffffffff {
		return nil
	}
	return unsafe.Pointer(uintptr(result))
}

//go:nosplit
func sysAllocOS(size uintptr, _ string) unsafe.Pointer {
	return kandeloMmap(nil, size, 3, kandeloMapPrivate|kandeloMapAnon)
}

//go:nosplit
func sysFreeOS(address unsafe.Pointer, size uintptr) {
	_, errno := doSyscall6(47, int64(uintptr(address)), int64(size), 0, 0, 0, 0)
	if errno != 0 {
		throw("Kandelo munmap failed")
	}
}

func sysUnusedOS(unsafe.Pointer, uintptr) {}

func sysUsedOS(unsafe.Pointer, uintptr) {}

func sysHugePageOS(unsafe.Pointer, uintptr) {}

func sysNoHugePageOS(unsafe.Pointer, uintptr) {}

func sysHugePageCollapseOS(unsafe.Pointer, uintptr) {}

func sysFaultOS(address unsafe.Pointer, size uintptr) {
	if kandeloMmap(address, size, 0, kandeloMapPrivate|kandeloMapAnon|kandeloMapFixed) != address {
		throw("Kandelo cannot fault memory")
	}
}

func sysReserveOS(address unsafe.Pointer, size uintptr, _ string) unsafe.Pointer {
	return kandeloMmap(address, size, 0, kandeloMapPrivate|kandeloMapAnon)
}

func sysMapOS(address unsafe.Pointer, size uintptr, _ string) {
	if kandeloMmap(address, size, 3, kandeloMapPrivate|kandeloMapAnon|kandeloMapFixed) != address {
		throw("Kandelo cannot map reserved memory")
	}
}

func resetMemoryDataView() {
	// This function is a no-op on Kandelo, it is only used to notify the
	// browser that its view of the WASM memory needs to be updated when
	// compiling for GOOS=js.
}

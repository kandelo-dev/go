//go:build kandelo && cgo

package runtime

import "unsafe"

var cgoCStack [8 << 20]byte
var kandeloCgoBootstrapStackBase uintptr
var kandeloCgoBootstrapSlotCount uint32 = 1

const kandeloCgoBootstrapStackSize = 32 << 10

func kandeloCgoReadChannelBase(base *uint32)

//go:nosplit
func kandeloCgoChannelBase() uint32 {
	base := &getg().m.cgoChannelBase
	kandeloCgoReadChannelBase(base)
	return *base
}

//go:nosplit
func kandeloCgoStackBounds(sp uintptr) (uintptr, uintptr) {
	base := kandeloCgoBootstrapStackBase
	end := base + uintptr(kandeloCgoBootstrapSlotCount)*kandeloCgoBootstrapStackSize
	if sp < base || sp >= end {
		return 0, 0
	}
	index := (sp - base) / kandeloCgoBootstrapStackSize
	lo := base + index*kandeloCgoBootstrapStackSize
	return lo, lo + kandeloCgoBootstrapStackSize
}

//go:linkname kandeloCgoStartupArgs
var kandeloCgoStartupArgs [2]uint32

//go:linkname kandeloCgoPrepareEnv
func kandeloCgoPrepareEnv() {
	if kandeloCgoBootstrapSlotCount == 0 || kandeloCgoBootstrapSlotCount > 1024 {
		throw("invalid cgo bootstrap stack count")
	}
	kandeloCgoBootstrapStackBase = uintptr(persistentalloc(uintptr(kandeloCgoBootstrapSlotCount)*kandeloCgoBootstrapStackSize, 16, &memstats.other_sys))
	if len(envs) > 4096 {
		throw("too many C environment entries")
	}
	programName := "a.out"
	if len(argslice) > 0 {
		programName = argslice[0]
	}
	total := uintptr((len(envs)+1)*4 + len(programName) + 1)
	for _, entry := range envs {
		total += uintptr(len(entry) + 1)
	}
	if total > 4<<20 {
		throw("C startup environment exceeds ARG_MAX")
	}
	allocation := uintptr(persistentalloc(total, 16, &memstats.other_sys))
	environment := allocation
	cursor := environment + uintptr((len(envs)+1)*4)
	kandeloCgoStartupArgs[0] = uint32(environment)
	kandeloCgoStartupArgs[1] = uint32(cursor)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(cursor)), len(programName)), programName)
	cursor += uintptr(len(programName) + 1)
	for index, entry := range envs {
		*(*uint32)(unsafe.Pointer(environment + uintptr(index)*4)) = uint32(cursor)
		copy(unsafe.Slice((*byte)(unsafe.Pointer(cursor)), len(entry)), entry)
		cursor += uintptr(len(entry) + 1)
	}
}

func cgoKandeloThreadInit(threadPointer unsafe.Pointer)

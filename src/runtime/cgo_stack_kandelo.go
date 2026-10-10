//go:build kandelo && cgo

package runtime

import "unsafe"

var cgoCStack [8 << 20]byte

//go:linkname kandeloCgoStartupArgs
var kandeloCgoStartupArgs [2]uint32

//go:linkname kandeloCgoPrepareEnv
func kandeloCgoPrepareEnv() {
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

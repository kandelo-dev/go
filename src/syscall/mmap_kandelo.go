package syscall

import (
	"runtime"
	"sync"
	"unsafe"
)

const (
	PROT_NONE  = 0
	PROT_READ  = 1
	PROT_WRITE = 2

	MAP_SHARED    = 0x01
	MAP_PRIVATE   = 0x02
	MAP_FIXED     = 0x10
	MAP_ANON      = 0x20
	MAP_ANONYMOUS = MAP_ANON
	MS_SYNC       = 4
	MADV_NORMAL   = 0
	MADV_RANDOM   = 1
)

var kandeloMappings = struct {
	sync.Mutex
	lengths map[uintptr]int
}{lengths: make(map[uintptr]int)}

func Mmap(fd int, offset int64, length int, prot int, flags int) ([]byte, error) {
	if length <= 0 {
		return nil, EINVAL
	}
	address, errno := kandeloSyscall6(kSysMmap, 0, int64(length), int64(prot), int64(flags), int64(fd), offset)
	if errno != 0 {
		return nil, kandeloErrno(errno)
	}
	if address <= 0 {
		return nil, EINVAL
	}
	data := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(address))), length)
	kandeloMappings.Lock()
	kandeloMappings.lengths[uintptr(address)] = length
	kandeloMappings.Unlock()
	return data, nil
}

func Munmap(data []byte) error {
	if len(data) == 0 || len(data) != cap(data) {
		return EINVAL
	}
	address := uintptr(unsafe.Pointer(&data[0]))
	kandeloMappings.Lock()
	defer kandeloMappings.Unlock()
	if kandeloMappings.lengths[address] != len(data) {
		return EINVAL
	}
	_, errno := kandeloSyscall6(kSysMunmap, int64(address), int64(len(data)), 0, 0, 0, 0)
	if errno != 0 {
		return kandeloErrno(errno)
	}
	delete(kandeloMappings.lengths, address)
	return nil
}

func Msync(data []byte, flags int) error {
	if len(data) == 0 {
		return EINVAL
	}
	address := uintptr(unsafe.Pointer(&data[0]))
	_, errno := kandeloSyscall6(kSysMsync, int64(address), int64(len(data)), int64(flags), 0, 0, 0)
	runtime.KeepAlive(data)
	return errnoErr(kandeloErrno(errno))
}

func Madvise(data []byte, advice int) error {
	if len(data) == 0 {
		return EINVAL
	}
	address := uintptr(unsafe.Pointer(&data[0]))
	_, errno := kandeloSyscall6(kSysMadvise, int64(address), int64(len(data)), int64(advice), 0, 0, 0)
	runtime.KeepAlive(data)
	return errnoErr(kandeloErrno(errno))
}

func Mlock(data []byte) error {
	address := uintptr(unsafe.Pointer(unsafe.SliceData(data)))
	_, errno := kandeloSyscall6(kSysMlock, int64(address), int64(len(data)), 0, 0, 0, 0)
	runtime.KeepAlive(data)
	return errnoErr(kandeloErrno(errno))
}

func Munlock(data []byte) error {
	address := uintptr(unsafe.Pointer(unsafe.SliceData(data)))
	_, errno := kandeloSyscall6(kSysMunlock, int64(address), int64(len(data)), 0, 0, 0, 0)
	runtime.KeepAlive(data)
	return errnoErr(kandeloErrno(errno))
}

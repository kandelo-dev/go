//go:build kandelo

package runtime

import (
	"internal/runtime/atomic"
	"unsafe"
)

const (
	kandeloEpollCreate1 = 239
	kandeloEpollCtl     = 240
	kandeloEpollPwait   = 241
	kandeloEventfd2     = 242
	kandeloRead         = 3
	kandeloWrite        = 4
	kandeloEpollIn      = 1
	kandeloEpollOut     = 4
	kandeloEpollErr     = 8
	kandeloEpollHup     = 16
	kandeloEpollAdd     = 1
	kandeloEpollDel     = 2
	kandeloEpollMod     = 3
	kandeloCloexec      = 0o2000000
	kandeloNonblock     = 0o4000
	kandeloArmedRead    = 1
	kandeloArmedWrite   = 2
	kandeloENOENT       = 2
	kandeloEINTR        = 4
	kandeloEAGAIN       = 11
)

type kandeloEpollEvent struct {
	events uint32
	_      uint32
	data   uint64
}

var (
	kandeloPollFd    int32
	kandeloWakeFd    int32
	kandeloPollLock  mutex
	kandeloWakeReady atomic.Uint32
)

func kandeloPollCtl(op int32, fd uintptr, event *kandeloEpollEvent) int32 {
	_, errno := doSyscall6(kandeloEpollCtl, int64(kandeloPollFd), int64(op), int64(fd), int64(uintptr(unsafe.Pointer(event))), 0, 0)
	return errno
}

func netpollinit() {
	fd, errno := doSyscall6(kandeloEpollCreate1, kandeloCloexec, 0, 0, 0, 0, 0)
	if errno != 0 {
		println("runtime: epoll_create1 failed with", errno)
		throw("runtime: netpollinit failed")
	}
	kandeloPollFd = int32(fd)
	fd, errno = doSyscall6(kandeloEventfd2, 0, kandeloCloexec|kandeloNonblock, 0, 0, 0, 0)
	if errno != 0 {
		println("runtime: eventfd2 failed with", errno)
		throw("runtime: netpollinit failed")
	}
	kandeloWakeFd = int32(fd)
	event := kandeloEpollEvent{events: kandeloEpollIn}
	if errno := kandeloPollCtl(kandeloEpollAdd, uintptr(kandeloWakeFd), &event); errno != 0 {
		println("runtime: epoll_ctl wake fd failed with", errno)
		throw("runtime: netpollinit failed")
	}
}

func netpollIsPollDescriptor(fd uintptr) bool {
	return netpollInited.Load() != 0 && (fd == uintptr(kandeloPollFd) || fd == uintptr(kandeloWakeFd))
}

func netpollopen(fd uintptr, pd *pollDesc) int32 {
	lock(&kandeloPollLock)
	pd.user = 0
	unlock(&kandeloPollLock)
	return 0
}

func netpollclose(fd uintptr) int32 {
	lock(&kandeloPollLock)
	errno := kandeloPollCtl(kandeloEpollDel, fd, nil)
	unlock(&kandeloPollLock)
	if errno == kandeloENOENT {
		return 0
	}
	return errno
}

func netpollarm(pd *pollDesc, mode int) {
	var armed uint32
	if mode == 'r' {
		armed = kandeloArmedRead
	} else {
		armed = kandeloArmedWrite
	}
	lock(&kandeloPollLock)
	if pd.user&armed != 0 {
		unlock(&kandeloPollLock)
		return
	}
	previous := pd.user
	armed |= previous
	event := kandeloEpollEvent{data: uint64(taggedPointerPack(unsafe.Pointer(pd), pd.fdseq.Load()))}
	if armed&kandeloArmedRead != 0 {
		event.events |= kandeloEpollIn
	}
	if armed&kandeloArmedWrite != 0 {
		event.events |= kandeloEpollOut
	}
	op := int32(kandeloEpollMod)
	if previous == 0 {
		op = kandeloEpollAdd
	}
	if errno := kandeloPollCtl(op, pd.fd, &event); errno != 0 {
		println("runtime: epoll_ctl arm failed with", errno)
		throw("runtime: netpollarm failed")
	}
	pd.user = armed
	unlock(&kandeloPollLock)
	netpollBreak()
}

func netpollBreak() {
	if netpollInited.Load() == 0 {
		return
	}
	if !kandeloWakeReady.CompareAndSwap(0, 1) {
		return
	}
	var value uint64 = 1
	for {
		_, errno := doSyscall6(kandeloWrite, int64(kandeloWakeFd), int64(uintptr(unsafe.Pointer(&value))), 8, 0, 0, 0)
		if errno == 0 || errno == kandeloEAGAIN {
			return
		}
		if errno != kandeloEINTR {
			println("runtime: eventfd write failed with", errno)
			throw("runtime: netpollBreak failed")
		}
	}
}

func netpoll(delay int64) (gList, int32) {
	if netpollInited.Load() == 0 {
		return gList{}, 0
	}
	var waitms int64
	if delay < 0 {
		waitms = -1
	} else if delay == 0 {
		waitms = 0
	} else if delay < 1e6 {
		waitms = 1
	} else if delay < 1e15 {
		waitms = (delay + 1e6 - 1) / 1e6
	} else {
		waitms = 1e9
	}
	var events [128]kandeloEpollEvent
	for {
		count, errno := doSyscall6(kandeloEpollPwait, int64(kandeloPollFd), int64(uintptr(unsafe.Pointer(&events[0]))), int64(len(events)), waitms, 0, 0)
		if errno == kandeloEINTR {
			if waitms > 0 {
				return gList{}, 0
			}
			continue
		}
		if errno != 0 {
			println("runtime: epoll_pwait failed with", errno)
			throw("runtime: netpoll failed")
		}
		var toRun gList
		var delta int32
		for index := int64(0); index < count; index++ {
			event := events[index]
			if event.data == 0 {
				if delay != 0 {
					var value uint64
					_, errno := doSyscall6(kandeloRead, int64(kandeloWakeFd), int64(uintptr(unsafe.Pointer(&value))), 8, 0, 0, 0)
					if errno != 0 && errno != kandeloEAGAIN {
						println("runtime: eventfd read failed with", errno)
						throw("runtime: netpoll failed")
					}
					kandeloWakeReady.Store(0)
				}
				continue
			}
			tagged := taggedPointer(event.data)
			pd := (*pollDesc)(tagged.pointer())
			lock(&kandeloPollLock)
			if pd.fdseq.Load() == tagged.tag() {
				var armed uint32
				if event.events&(kandeloEpollIn|kandeloEpollErr|kandeloEpollHup) != 0 {
					armed |= kandeloArmedRead
				}
				if event.events&(kandeloEpollOut|kandeloEpollErr|kandeloEpollHup) != 0 {
					armed |= kandeloArmedWrite
				}
				armed &= pd.user
				if armed != 0 {
					pd.user &^= armed
					next := kandeloEpollEvent{data: event.data}
					if pd.user&kandeloArmedRead != 0 {
						next.events |= kandeloEpollIn
					}
					if pd.user&kandeloArmedWrite != 0 {
						next.events |= kandeloEpollOut
					}
					op := int32(kandeloEpollMod)
					if pd.user == 0 {
						op = kandeloEpollDel
					}
					if errno := kandeloPollCtl(op, pd.fd, &next); errno != 0 {
						println("runtime: epoll_ctl disarm failed with", errno)
						throw("runtime: netpoll failed")
					}
					var mode int32
					if armed&kandeloArmedRead != 0 {
						mode += 'r'
					}
					if armed&kandeloArmedWrite != 0 {
						mode += 'w'
					}
					pd.setEventErr(event.events&kandeloEpollErr != 0, tagged.tag())
					delta += netpollready(&toRun, pd, mode)
				}
			}
			unlock(&kandeloPollLock)
		}
		return toRun, delta
	}
}

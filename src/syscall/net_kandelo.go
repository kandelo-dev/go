// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build kandelo

package syscall

import (
	"runtime"
	"unsafe"
)

const (
	SHUT_RD   = 0
	SHUT_WR   = 1
	SHUT_RDWR = 2
)

func socketAddressBytes(sa Sockaddr) ([]byte, error) {
	switch address := sa.(type) {
	case *SockaddrInet4:
		if address == nil {
			return nil, EINVAL
		}
		if address.Port < 0 || address.Port > 65535 {
			return nil, EINVAL
		}
		data := make([]byte, 16)
		data[0] = AF_INET
		data[2] = byte(address.Port >> 8)
		data[3] = byte(address.Port)
		copy(data[4:8], address.Addr[:])
		return data, nil
	case *SockaddrInet6:
		if address == nil {
			return nil, EINVAL
		}
		if address.Port < 0 || address.Port > 65535 {
			return nil, EINVAL
		}
		data := make([]byte, 28)
		data[0] = AF_INET6
		data[2] = byte(address.Port >> 8)
		data[3] = byte(address.Port)
		copy(data[8:24], address.Addr[:])
		data[24] = byte(address.ZoneId)
		data[25] = byte(address.ZoneId >> 8)
		data[26] = byte(address.ZoneId >> 16)
		data[27] = byte(address.ZoneId >> 24)
		return data, nil
	default:
		return nil, EAFNOSUPPORT
	}
}

func socketAddressFromBytes(data []byte) (Sockaddr, error) {
	if len(data) < 2 {
		return nil, EINVAL
	}
	family := int(data[0]) | int(data[1])<<8
	switch family {
	case AF_INET:
		if len(data) < 16 {
			return nil, EINVAL
		}
		address := &SockaddrInet4{Port: int(data[2])<<8 | int(data[3])}
		copy(address.Addr[:], data[4:8])
		return address, nil
	case AF_INET6:
		if len(data) < 28 {
			return nil, EINVAL
		}
		address := &SockaddrInet6{Port: int(data[2])<<8 | int(data[3])}
		copy(address.Addr[:], data[8:24])
		address.ZoneId = uint32(data[24]) | uint32(data[25])<<8 | uint32(data[26])<<16 | uint32(data[27])<<24
		return address, nil
	default:
		return nil, EAFNOSUPPORT
	}
}

func socketAddressName(fd int, syscallNumber int32) (Sockaddr, error) {
	var data [128]byte
	length := uint32(len(data))
	_, errno := kandeloSyscall6(syscallNumber, int64(fd), int64(uintptr(unsafe.Pointer(&data[0]))), int64(uintptr(unsafe.Pointer(&length))), 0, 0, 0)
	if errno != 0 {
		return nil, kandeloErrno(errno)
	}
	if length > uint32(len(data)) {
		return nil, EINVAL
	}
	return socketAddressFromBytes(data[:length])
}

func Socket(domain, socketType, protocol int) (fd int, err error) {
	ret, errno := kandeloSyscall6(kSysSocket, int64(domain), int64(socketType), int64(protocol), 0, 0, 0)
	return int(ret), errnoErr(kandeloErrno(errno))
}

func Bind(fd int, sa Sockaddr) error {
	address, err := socketAddressBytes(sa)
	if err != nil {
		return err
	}
	_, errno := kandeloSyscall6(kSysBind, int64(fd), int64(uintptr(unsafe.Pointer(unsafe.SliceData(address)))), int64(len(address)), 0, 0, 0)
	runtime.KeepAlive(address)
	return errnoErr(kandeloErrno(errno))
}

func StopIO(fd int) error {
	return Shutdown(fd, SHUT_RDWR)
}

func Listen(fd int, backlog int) error {
	_, errno := kandeloSyscall6(kSysListen, int64(fd), int64(backlog), 0, 0, 0, 0)
	return errnoErr(kandeloErrno(errno))
}

func Accept(fd int) (int, Sockaddr, error) {
	var address [128]byte
	length := uint32(len(address))
	ret, errno := kandeloSyscall6(kSysAccept, int64(fd), int64(uintptr(unsafe.Pointer(&address[0]))), int64(uintptr(unsafe.Pointer(&length))), 0, 0, 0)
	if errno != 0 {
		return 0, nil, kandeloErrno(errno)
	}
	if length > uint32(len(address)) {
		Close(int(ret))
		return 0, nil, EINVAL
	}
	sa, err := socketAddressFromBytes(address[:length])
	if err != nil {
		Close(int(ret))
		return 0, nil, err
	}
	return int(ret), sa, nil
}

func Connect(fd int, sa Sockaddr) error {
	address, err := socketAddressBytes(sa)
	if err != nil {
		return err
	}
	_, errno := kandeloSyscall6(kSysConnect, int64(fd), int64(uintptr(unsafe.Pointer(unsafe.SliceData(address)))), int64(len(address)), 0, 0, 0)
	runtime.KeepAlive(address)
	return errnoErr(kandeloErrno(errno))
}

func Getsockname(fd int) (Sockaddr, error) { return socketAddressName(fd, kSysGetsockname) }

func Getpeername(fd int) (Sockaddr, error) { return socketAddressName(fd, kSysGetpeername) }

func Recvfrom(fd int, p []byte, flags int) (n int, from Sockaddr, err error) {
	return 0, nil, ENOSYS
}

func Sendto(fd int, p []byte, flags int, to Sockaddr) error {
	return ENOSYS
}

func Recvmsg(fd int, p, oob []byte, flags int) (n, oobn, recvflags int, from Sockaddr, err error) {
	return 0, 0, 0, nil, ENOSYS
}

func SendmsgN(fd int, p, oob []byte, to Sockaddr, flags int) (n int, err error) {
	return 0, ENOSYS
}

func GetsockoptInt(fd, level, opt int) (value int, err error) {
	var result int32
	length := uint32(4)
	_, errno := kandeloSyscall6(kSysGetsockopt, int64(fd), int64(level), int64(opt), int64(uintptr(unsafe.Pointer(&result))), int64(uintptr(unsafe.Pointer(&length))), 0)
	if errno != 0 {
		return 0, kandeloErrno(errno)
	}
	if length != 4 {
		return 0, EINVAL
	}
	return int(result), nil
}

func SetsockoptInt(fd, level, opt int, value int) error {
	option := int32(value)
	_, errno := kandeloSyscall6(kSysSetsockopt, int64(fd), int64(level), int64(opt), int64(uintptr(unsafe.Pointer(&option))), 4, 0)
	return errnoErr(kandeloErrno(errno))
}

func SetsockoptLinger(fd, level, opt int, value *Linger) error {
	if value == nil {
		return EINVAL
	}
	_, errno := kandeloSyscall6(kSysSetsockopt, int64(fd), int64(level), int64(opt), int64(uintptr(unsafe.Pointer(value))), int64(unsafe.Sizeof(*value)), 0)
	runtime.KeepAlive(value)
	return errnoErr(kandeloErrno(errno))
}

func SetsockoptInet4Addr(fd, level, opt int, value [4]byte) error {
	_, errno := kandeloSyscall6(kSysSetsockopt, int64(fd), int64(level), int64(opt), int64(uintptr(unsafe.Pointer(&value[0]))), int64(len(value)), 0)
	return errnoErr(kandeloErrno(errno))
}

func SetReadDeadline(fd int, t int64) error {
	return ENOSYS
}

func SetWriteDeadline(fd int, t int64) error {
	return ENOSYS
}

func Shutdown(fd int, how int) error {
	_, errno := kandeloSyscall6(kSysShutdown, int64(fd), int64(how), 0, 0, 0, 0)
	return errnoErr(kandeloErrno(errno))
}

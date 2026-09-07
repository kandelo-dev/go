// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build kandelo

// GOOS=kandelo mirrors the wasip1 syscall surface for milestone 1, but every
// host syscall backend here is a STUB: the wasip1 //go:wasmimport leaves have
// been replaced with functions returning ENOSYS (see the one-line bodies
// below). This lets the syscall package compile for kandelo without importing
// any host functions. The real Kandelo syscall backend (a host channel
// handshake) is a later milestone task.

package syscall

const (
	SHUT_RD   = 0x1
	SHUT_WR   = 0x2
	SHUT_RDWR = SHUT_RD | SHUT_WR
)

type sdflags = uint32

func sock_accept(fd int32, flags fdflags, newfd *int32) Errno { return ENOSYS }

func sock_shutdown(fd int32, flags sdflags) Errno { return ENOSYS }

func Socket(proto, sotype, unused int) (fd int, err error) {
	return 0, ENOSYS
}

func Bind(fd int, sa Sockaddr) error {
	return ENOSYS
}

func StopIO(fd int) error {
	return ENOSYS
}

func Listen(fd int, backlog int) error {
	return ENOSYS
}

func Accept(fd int) (int, Sockaddr, error) {
	var newfd int32
	errno := sock_accept(int32(fd), 0, &newfd)
	return int(newfd), nil, errnoErr(errno)
}

func Connect(fd int, sa Sockaddr) error {
	return ENOSYS
}

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
	return 0, ENOSYS
}

func SetsockoptInt(fd, level, opt int, value int) error {
	return ENOSYS
}

func SetReadDeadline(fd int, t int64) error {
	return ENOSYS
}

func SetWriteDeadline(fd int, t int64) error {
	return ENOSYS
}

func Shutdown(fd int, how int) error {
	errno := sock_shutdown(int32(fd), sdflags(how))
	return errnoErr(errno)
}

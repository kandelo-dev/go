// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build kandelo

package syscall

import "runtime"

// GOOS=kandelo speaks the kernel's Linux/musl errno numbering: the syscall
// channel returns the same errno values musl's generic bits/errno.h uses (see
// libc/musl/arch/generic/bits/errno.h in the Kandelo repository), so these
// constants mirror those numbers exactly. That lets the channel's errno slot
// pass through as a syscall.Errno without translation. Earlier milestone-1
// code used WASI's errno numbering, which does not match what the Kandelo
// kernel emits.
const (
	EPERM           Errno = 1
	ENOENT          Errno = 2
	ESRCH           Errno = 3
	EINTR           Errno = 4
	EIO             Errno = 5
	ENXIO           Errno = 6
	E2BIG           Errno = 7
	ENOEXEC         Errno = 8
	EBADF           Errno = 9
	ECHILD          Errno = 10
	EAGAIN          Errno = 11
	ENOMEM          Errno = 12
	EACCES          Errno = 13
	EFAULT          Errno = 14
	EBUSY           Errno = 16
	EEXIST          Errno = 17
	EXDEV           Errno = 18
	ENODEV          Errno = 19
	ENOTDIR         Errno = 20
	EISDIR          Errno = 21
	EINVAL          Errno = 22
	ENFILE          Errno = 23
	EMFILE          Errno = 24
	ENOTTY          Errno = 25
	ETXTBSY         Errno = 26
	EFBIG           Errno = 27
	ENOSPC          Errno = 28
	ESPIPE          Errno = 29
	EROFS           Errno = 30
	EMLINK          Errno = 31
	EPIPE           Errno = 32
	EDOM            Errno = 33
	ERANGE          Errno = 34
	EDEADLK         Errno = 35
	ENAMETOOLONG    Errno = 36
	ENOLCK          Errno = 37
	ENOSYS          Errno = 38
	ENOTEMPTY       Errno = 39
	ELOOP           Errno = 40
	ENOMSG          Errno = 42
	EIDRM           Errno = 43
	ENOLINK         Errno = 67
	EPROTO          Errno = 71
	EMULTIHOP       Errno = 72
	EBADMSG         Errno = 74
	EOVERFLOW       Errno = 75
	EBADFD          Errno = 77
	EILSEQ          Errno = 84
	ENOTSOCK        Errno = 88
	EDESTADDRREQ    Errno = 89
	EMSGSIZE        Errno = 90
	EPROTOTYPE      Errno = 91
	ENOPROTOOPT     Errno = 92
	EPROTONOSUPPORT Errno = 93
	EOPNOTSUPP      Errno = 95
	ENOTSUP         Errno = EOPNOTSUPP
	EAFNOSUPPORT    Errno = 97
	EADDRINUSE      Errno = 98
	EADDRNOTAVAIL   Errno = 99
	ENETDOWN        Errno = 100
	ENETUNREACH     Errno = 101
	ENETRESET       Errno = 102
	ECONNABORTED    Errno = 103
	ECONNRESET      Errno = 104
	ENOBUFS         Errno = 105
	EISCONN         Errno = 106
	ENOTCONN        Errno = 107
	ETIMEDOUT       Errno = 110
	ECONNREFUSED    Errno = 111
	EHOSTUNREACH    Errno = 113
	EALREADY        Errno = 114
	EINPROGRESS     Errno = 115
	ESTALE          Errno = 116
	EDQUOT          Errno = 122
	ECANCELED       Errno = 125
	EOWNERDEAD      Errno = 130
	ENOTRECOVERABLE Errno = 131

	EWOULDBLOCK Errno = EAGAIN

	// ENOTCAPABLE is a WASI concept with no Linux/musl equivalent. It is not
	// emitted by the Kandelo kernel; it is defined only so code shared with the
	// wasip1 port still compiles. Use a sentinel outside the errno range.
	ENOTCAPABLE Errno = 0x7fff
)

// errorstr is indexed by Errno; it uses the Linux/musl numbering above.
var errorstr = [...]string{
	EPERM:           "Operation not permitted",
	ENOENT:          "No such file or directory",
	ESRCH:           "No such process",
	EINTR:           "Interrupted system call",
	EIO:             "I/O error",
	ENXIO:           "No such device or address",
	E2BIG:           "Argument list too long",
	ENOEXEC:         "Exec format error",
	EBADF:           "Bad file number",
	ECHILD:          "No child processes",
	EAGAIN:          "Try again",
	ENOMEM:          "Out of memory",
	EACCES:          "Permission denied",
	EFAULT:          "Bad address",
	EBUSY:           "Device or resource busy",
	EEXIST:          "File exists",
	EXDEV:           "Cross-device link",
	ENODEV:          "No such device",
	ENOTDIR:         "Not a directory",
	EISDIR:          "Is a directory",
	EINVAL:          "Invalid argument",
	ENFILE:          "File table overflow",
	EMFILE:          "Too many open files",
	ENOTTY:          "Not a typewriter",
	ETXTBSY:         "Text file busy",
	EFBIG:           "File too large",
	ENOSPC:          "No space left on device",
	ESPIPE:          "Illegal seek",
	EROFS:           "Read-only file system",
	EMLINK:          "Too many links",
	EPIPE:           "Broken pipe",
	EDOM:            "Math arg out of domain of func",
	ERANGE:          "Math result not representable",
	EDEADLK:         "Deadlock condition",
	ENAMETOOLONG:    "File name too long",
	ENOLCK:          "No record locks available",
	ENOSYS:          "Not implemented on " + runtime.GOOS,
	ENOTEMPTY:       "Directory not empty",
	ELOOP:           "Too many symbolic links",
	ENOMSG:          "No message of desired type",
	EIDRM:           "Identifier removed",
	ENOLINK:         "The link has been severed",
	EPROTO:          "Protocol error",
	EMULTIHOP:       "Multihop attempted",
	EBADMSG:         "Trying to read unreadable message",
	EOVERFLOW:       "Value too large for defined data type",
	EBADFD:          "file descriptor in bad state",
	EILSEQ:          "EILSEQ",
	ENOTSOCK:        "Socket operation on non-socket",
	EDESTADDRREQ:    "Destination address required",
	EMSGSIZE:        "Message too long",
	EPROTOTYPE:      "Protocol wrong type for socket",
	ENOPROTOOPT:     "Protocol not available",
	EPROTONOSUPPORT: "Unknown protocol",
	EOPNOTSUPP:      "Operation not supported",
	EAFNOSUPPORT:    "Address family not supported by protocol family",
	EADDRINUSE:      "Address already in use",
	EADDRNOTAVAIL:   "Address not available",
	ENETDOWN:        "Network interface is not configured",
	ENETUNREACH:     "Network is unreachable",
	ENETRESET:       "Network dropped connection on reset",
	ECONNABORTED:    "Connection aborted",
	ECONNRESET:      "Connection reset by peer",
	ENOBUFS:         "No buffer space available",
	EISCONN:         "Socket is already connected",
	ENOTCONN:        "Socket is not connected",
	ETIMEDOUT:       "Connection timed out",
	ECONNREFUSED:    "Connection refused",
	EHOSTUNREACH:    "Host is unreachable",
	EALREADY:        "Socket already connected",
	EINPROGRESS:     "Connection already in progress",
	ESTALE:          "Stale file handle",
	EDQUOT:          "Quota exceeded",
	ECANCELED:       "Operation canceled",
	EOWNERDEAD:      "Owner died",
	ENOTRECOVERABLE: "State not recoverable",
}

// Do the interface allocations only once for common
// Errno values.
var (
	errEAGAIN error = EAGAIN
	errEINVAL error = EINVAL
	errENOENT error = ENOENT
)

// errnoErr returns common boxed Errno values, to prevent
// allocations at runtime.
//
//go:noinline
//go:nosplit
func errnoErr(e Errno) error {
	switch e {
	case 0:
		return nil
	case EAGAIN:
		return errEAGAIN
	case EINVAL:
		return errEINVAL
	case ENOENT:
		return errENOENT
	}
	return e
}

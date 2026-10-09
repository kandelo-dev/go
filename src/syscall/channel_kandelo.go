// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build kandelo

package syscall

// This file gives the syscall package direct access to the Kandelo syscall
// channel. The runtime owns the atomic channel handshake (see
// runtime/channel_kandelo.go); it exposes the six-argument entry point below
// via //go:linkname so os and internal/poll can perform real file I/O through
// the same channel the runtime already uses for the runtime's own write1.
//
// kandeloSyscall6 has no body here: the runtime supplies it through
//
//	//go:linkname syscall_kandeloSyscall6 syscall.kandeloSyscall6
//
// exactly as os_wasm.go supplies syscall.now. It returns the kernel's raw
// (return value, errno) pair, where errno uses the Linux/musl numbering
// mirrored in tables_kandelo.go.
func kandeloSyscall6(number int32, a0, a1, a2, a3, a4, a5 int64) (ret int64, errno int32)

// Kandelo kernel syscall numbers. These match crates/shared/src/lib.rs
// (enum Syscall / mod extended_syscalls) and the musl overlay's bits/syscall.h;
// they are the kernel's own numbering, not the classic Linux amd64 numbers.
const (
	kSysOpen        int32 = 1
	kSysClose       int32 = 2
	kSysRead        int32 = 3
	kSysWrite       int32 = 4
	kSysDup         int32 = 7
	kSysDup2        int32 = 8
	kSysPipe        int32 = 9
	kSysFcntl       int32 = 10
	kSysLseek       int32 = 5
	kSysFstat       int32 = 6
	kSysStat        int32 = 11
	kSysChmod       int32 = 20
	kSysGetcwd      int32 = 23
	kSysChdir       int32 = 24
	kSysGetpid      int32 = 28
	kSysGetppid     int32 = 29
	kSysKill        int32 = 35
	kSysIsatty      int32 = 42
	kSysSocket      int32 = 50
	kSysBind        int32 = 51
	kSysListen      int32 = 52
	kSysAccept      int32 = 53
	kSysConnect     int32 = 54
	kSysShutdown    int32 = 57
	kSysGetsockopt  int32 = 58
	kSysSetsockopt  int32 = 59
	kSysPread       int32 = 64
	kSysPwrite      int32 = 65
	kSysOpenat      int32 = 69
	kSysPipe2       int32 = 78
	kSysFtruncate   int32 = 79
	kSysFsync       int32 = 80
	kSysTruncate    int32 = 85
	kSysFchmod      int32 = 87
	kSysFstatat     int32 = 93
	kSysUnlinkat    int32 = 94
	kSysMkdirat     int32 = 95
	kSysRenameat    int32 = 96
	kSysFchmodat    int32 = 98
	kSysLinkat      int32 = 100
	kSysSymlinkat   int32 = 101
	kSysReadlinkat  int32 = 102
	kSysGetsockname int32 = 114
	kSysGetpeername int32 = 115
	kSysGetrandom   int32 = 120
	kSysUtimensat   int32 = 125
	kSysFchdir      int32 = 127
	kSysWait4       int32 = 139
	kSysSpawn       int32 = 500
)

// kAtFdcwd is the dirfd sentinel meaning "resolve relative to the current
// working directory", matching musl's AT_FDCWD on this arch.
const kAtFdcwd int32 = -100

// kAtRemovedir is the unlinkat(2) flag selecting directory removal (rmdir),
// matching Linux/musl AT_REMOVEDIR.
const kAtRemovedir int32 = 0x200

// kandeloErrno converts a raw channel errno into a syscall.Errno, mapping the
// runtime's "no channel base" ENOSYS sentinel (38) through unchanged since the
// numbering already matches.
func kandeloErrno(errno int32) Errno {
	return Errno(errno)
}

// Standard Linux/musl st_mode file-type bits. The S_IF* constants declared in
// syscall_kandelo.go carry WASI-era octal values that do not match the kernel's
// st_mode, so the fstat conversion uses these canonical masks instead.
const (
	kSIFMT   = 0o170000
	kSIFDIR  = 0o040000
	kSIFCHR  = 0o020000
	kSIFBLK  = 0o060000
	kSIFREG  = 0o100000
	kSIFIFO  = 0o010000
	kSIFLNK  = 0o120000
	kSIFSOCK = 0o140000
)

// kernelStat mirrors the 112-byte native struct stat the kernel writes for the
// fstat/stat/fstatat syscalls. The layout MUST match
// libc/musl/arch/wasm32posix/kstat.h and crates/shared/src/process_layout.rs.
type kernelStat struct {
	Dev       uint64
	Ino       uint64
	Mode      uint32
	Nlink     uint32
	Uid       uint32
	Gid       uint32
	Size      uint64
	AtimeSec  int64
	AtimeNsec uint32
	_         uint32
	MtimeSec  int64
	MtimeNsec uint32
	_         uint32
	CtimeSec  int64
	CtimeNsec uint32
	_         uint32
	Rdev      uint64
	Blksize   int32
	_         int32
	Blocks    int64
}

// toStat converts the native kernel stat into the Go Stat_t the os package
// consumes. Filetype is derived from st_mode using the canonical Linux masks,
// and the timestamps are flattened to nanoseconds (os builds time.Unix(0, ns)).
func (ks *kernelStat) toStat(st *Stat_t) {
	st.Dev = ks.Dev
	st.Ino = ks.Ino
	st.Nlink = uint64(ks.Nlink)
	st.Size = ks.Size
	st.Uid = ks.Uid
	st.Gid = ks.Gid
	st.Mode = int(ks.Mode)
	st.Atime = uint64(ks.AtimeSec)*1_000_000_000 + uint64(ks.AtimeNsec)
	st.Mtime = uint64(ks.MtimeSec)*1_000_000_000 + uint64(ks.MtimeNsec)
	st.Ctime = uint64(ks.CtimeSec)*1_000_000_000 + uint64(ks.CtimeNsec)

	switch ks.Mode & kSIFMT {
	case kSIFDIR:
		st.Filetype = FILETYPE_DIRECTORY
	case kSIFCHR:
		st.Filetype = FILETYPE_CHARACTER_DEVICE
	case kSIFBLK:
		st.Filetype = FILETYPE_BLOCK_DEVICE
	case kSIFREG:
		st.Filetype = FILETYPE_REGULAR_FILE
	case kSIFLNK:
		st.Filetype = FILETYPE_SYMBOLIC_LINK
	case kSIFSOCK:
		st.Filetype = FILETYPE_SOCKET_STREAM
	default:
		st.Filetype = FILETYPE_UNKNOWN
	}
}

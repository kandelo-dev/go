// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build kandelo

// GOOS=kandelo mirrors the wasip1 syscall surface, but instead of WASI
// //go:wasmimport leaves it routes syscalls through the Kandelo channel
// handshake (kandeloSyscall6 in channel_kandelo.go) using kernel/musl syscall
// numbers. The fd_* read/write/seek/close/stat leaves and the path-based
// Open/Openat/Stat/Lstat calls are implemented; some path operations
// (readdir, and the WASI path_* rights helpers) remain ENOSYS stubs pending
// later milestones.

package syscall

import (
	"internal/stringslite"
	"runtime"
	"structs"
	"unsafe"
)

func init() {
	// Try to set stdio to non-blocking mode before the os package
	// calls NewFile for each fd. NewFile queries the non-blocking flag
	// but doesn't change it, even if the runtime supports non-blocking
	// stdio. Since WebAssembly modules are single-threaded, blocking
	// system calls temporarily halt execution of the module. If the
	// runtime supports non-blocking stdio, the Go runtime is able to
	// use the WASI net poller to poll for read/write readiness and is
	// able to schedule goroutines while waiting.
	SetNonblock(0, true)
	SetNonblock(1, true)
	SetNonblock(2, true)
}

type uintptr32 = uint32
type size = uint32
type fdflags = uint32
type filesize = uint64
type filetype = uint8
type lookupflags = uint32
type oflags = uint32
type rights = uint64
type timestamp = uint64
type dircookie = uint64
type filedelta = int64
type fstflags = uint32

type iovec struct {
	_      structs.HostLayout
	buf    uintptr32
	bufLen size
}

const (
	LOOKUP_SYMLINK_FOLLOW = 0x00000001
)

const (
	OFLAG_CREATE    = 0x0001
	OFLAG_DIRECTORY = 0x0002
	OFLAG_EXCL      = 0x0004
	OFLAG_TRUNC     = 0x0008
)

const (
	FDFLAG_APPEND   = 0x0001
	FDFLAG_DSYNC    = 0x0002
	FDFLAG_NONBLOCK = 0x0004
	FDFLAG_RSYNC    = 0x0008
	FDFLAG_SYNC     = 0x0010
)

const (
	RIGHT_FD_DATASYNC = 1 << iota
	RIGHT_FD_READ
	RIGHT_FD_SEEK
	RIGHT_FDSTAT_SET_FLAGS
	RIGHT_FD_SYNC
	RIGHT_FD_TELL
	RIGHT_FD_WRITE
	RIGHT_FD_ADVISE
	RIGHT_FD_ALLOCATE
	RIGHT_PATH_CREATE_DIRECTORY
	RIGHT_PATH_CREATE_FILE
	RIGHT_PATH_LINK_SOURCE
	RIGHT_PATH_LINK_TARGET
	RIGHT_PATH_OPEN
	RIGHT_FD_READDIR
	RIGHT_PATH_READLINK
	RIGHT_PATH_RENAME_SOURCE
	RIGHT_PATH_RENAME_TARGET
	RIGHT_PATH_FILESTAT_GET
	RIGHT_PATH_FILESTAT_SET_SIZE
	RIGHT_PATH_FILESTAT_SET_TIMES
	RIGHT_FD_FILESTAT_GET
	RIGHT_FD_FILESTAT_SET_SIZE
	RIGHT_FD_FILESTAT_SET_TIMES
	RIGHT_PATH_SYMLINK
	RIGHT_PATH_REMOVE_DIRECTORY
	RIGHT_PATH_UNLINK_FILE
	RIGHT_POLL_FD_READWRITE
	RIGHT_SOCK_SHUTDOWN
	RIGHT_SOCK_ACCEPT
)

const (
	WHENCE_SET = 0
	WHENCE_CUR = 1
	WHENCE_END = 2
)

const (
	FILESTAT_SET_ATIM     = 0x0001
	FILESTAT_SET_ATIM_NOW = 0x0002
	FILESTAT_SET_MTIM     = 0x0004
	FILESTAT_SET_MTIM_NOW = 0x0008
)

const (
	// Despite the rights being defined as a 64 bits integer in the spec,
	// wasmtime crashes the program if we set any of the upper 32 bits.
	fullRights  = rights(^uint32(0))
	readRights  = rights(RIGHT_FD_READ | RIGHT_FD_READDIR)
	writeRights = rights(RIGHT_FD_DATASYNC | RIGHT_FD_WRITE | RIGHT_FD_ALLOCATE | RIGHT_PATH_FILESTAT_SET_SIZE)

	// Some runtimes have very strict expectations when it comes to which
	// rights can be enabled on files opened by path_open. The fileRights
	// constant is used as a mask to retain only bits for operations that
	// are supported on files.
	fileRights rights = RIGHT_FD_DATASYNC |
		RIGHT_FD_READ |
		RIGHT_FD_SEEK |
		RIGHT_FDSTAT_SET_FLAGS |
		RIGHT_FD_SYNC |
		RIGHT_FD_TELL |
		RIGHT_FD_WRITE |
		RIGHT_FD_ADVISE |
		RIGHT_FD_ALLOCATE |
		RIGHT_PATH_CREATE_DIRECTORY |
		RIGHT_PATH_CREATE_FILE |
		RIGHT_PATH_LINK_SOURCE |
		RIGHT_PATH_LINK_TARGET |
		RIGHT_PATH_OPEN |
		RIGHT_FD_READDIR |
		RIGHT_PATH_READLINK |
		RIGHT_PATH_RENAME_SOURCE |
		RIGHT_PATH_RENAME_TARGET |
		RIGHT_PATH_FILESTAT_GET |
		RIGHT_PATH_FILESTAT_SET_SIZE |
		RIGHT_PATH_FILESTAT_SET_TIMES |
		RIGHT_FD_FILESTAT_GET |
		RIGHT_FD_FILESTAT_SET_SIZE |
		RIGHT_FD_FILESTAT_SET_TIMES |
		RIGHT_PATH_SYMLINK |
		RIGHT_PATH_REMOVE_DIRECTORY |
		RIGHT_PATH_UNLINK_FILE |
		RIGHT_POLL_FD_READWRITE

	// Runtimes like wasmtime and wasmedge will refuse to open directories
	// if the rights requested by the application exceed the operations that
	// can be performed on a directory.
	dirRights rights = RIGHT_FD_SEEK |
		RIGHT_FDSTAT_SET_FLAGS |
		RIGHT_FD_SYNC |
		RIGHT_PATH_CREATE_DIRECTORY |
		RIGHT_PATH_CREATE_FILE |
		RIGHT_PATH_LINK_SOURCE |
		RIGHT_PATH_LINK_TARGET |
		RIGHT_PATH_OPEN |
		RIGHT_FD_READDIR |
		RIGHT_PATH_READLINK |
		RIGHT_PATH_RENAME_SOURCE |
		RIGHT_PATH_RENAME_TARGET |
		RIGHT_PATH_FILESTAT_GET |
		RIGHT_PATH_FILESTAT_SET_SIZE |
		RIGHT_PATH_FILESTAT_SET_TIMES |
		RIGHT_FD_FILESTAT_GET |
		RIGHT_FD_FILESTAT_SET_TIMES |
		RIGHT_PATH_SYMLINK |
		RIGHT_PATH_REMOVE_DIRECTORY |
		RIGHT_PATH_UNLINK_FILE
)

// iovecAt returns a pointer to the i-th iovec in the array starting at iovs.
//
//go:nosplit
func iovecAt(iovs *iovec, i size) *iovec {
	return (*iovec)(unsafe.Add(unsafe.Pointer(iovs), uintptr(i)*unsafe.Sizeof(iovec{})))
}

// fd_close routes to the kernel close(2) over the channel.
func fd_close(fd int32) Errno {
	_, errno := kandeloSyscall6(kSysClose, int64(fd), 0, 0, 0, 0, 0)
	return kandeloErrno(errno)
}

func fd_filestat_set_size(fd int32, set_size filesize) Errno {
	_, errno := kandeloSyscall6(kSysFtruncate, int64(fd), int64(set_size), 0, 0, 0, 0)
	return kandeloErrno(errno)
}

// fd_pread performs positioned reads by issuing one kernel pread(2) per iovec.
func fd_pread(fd int32, iovs *iovec, iovsLen size, offset filesize, nread *size) Errno {
	return channelReadv(kSysPread, fd, iovs, iovsLen, int64(offset), nread)
}

// fd_pwrite performs positioned writes by issuing one kernel pwrite(2) per iovec.
func fd_pwrite(fd int32, iovs *iovec, iovsLen size, offset filesize, nwritten *size) Errno {
	return channelWritev(kSysPwrite, fd, iovs, iovsLen, int64(offset), nwritten)
}

// fd_read scatters a read across the iovec array using kernel read(2).
func fd_read(fd int32, iovs *iovec, iovsLen size, nread *size) Errno {
	return channelReadv(kSysRead, fd, iovs, iovsLen, -1, nread)
}

func fd_readdir(fd int32, buf *byte, bufLen size, cookie dircookie, nwritten *size) Errno {
	return ENOSYS
}

func fd_seek(fd int32, offset filedelta, whence uint32, newoffset *filesize) Errno {
	ret, errno := kandeloSyscall6(kSysLseek, int64(fd), int64(offset), int64(whence), 0, 0, 0)
	if errno != 0 {
		return kandeloErrno(errno)
	}
	*newoffset = filesize(ret)
	return 0
}

// https://github.com/WebAssembly/WASI/blob/a2b96e81c0586125cc4dc79a5be0b78d9a059925/legacy/preview1/docs.md#-fd_fdstat_set_rightsfd-fd-fs_rights_base-rights-fs_rights_inheriting-rights---result-errno
func fd_fdstat_set_rights(fd int32, rightsBase rights, rightsInheriting rights) Errno { return ENOSYS }

// fd_filestat_get fills a Stat_t from the kernel fstat(2). The kernel writes a
// native struct stat directly into the target via the raw pointer, so this
// converts that into the Go Stat_t layout expected by the os package.
func fd_filestat_get(fd int32, buf unsafe.Pointer) Errno {
	var ks kernelStat
	_, errno := kandeloSyscall6(kSysFstat, int64(fd), int64(uintptr(unsafe.Pointer(&ks))), 0, 0, 0, 0)
	if errno != 0 {
		return kandeloErrno(errno)
	}
	ks.toStat((*Stat_t)(buf))
	return 0
}

// channelWritev writes each iovec through the channel using the given kernel
// syscall number (write or pwrite). offset < 0 selects the non-positioned form.
//
//go:nosplit
func channelWritev(number int32, fd int32, iovs *iovec, iovsLen size, offset int64, nwritten *size) Errno {
	var total size
	for i := size(0); i < iovsLen; i++ {
		iov := iovecAt(iovs, i)
		if iov.bufLen == 0 {
			continue
		}
		var ret int64
		var errno int32
		if offset < 0 {
			ret, errno = kandeloSyscall6(number, int64(fd), int64(iov.buf), int64(iov.bufLen), 0, 0, 0)
		} else {
			ret, errno = kandeloSyscall6(number, int64(fd), int64(iov.buf), int64(iov.bufLen), offset, 0, 0)
		}
		if errno != 0 {
			if total != 0 {
				break
			}
			return kandeloErrno(errno)
		}
		total += size(ret)
		if offset >= 0 {
			offset += ret
		}
		if size(ret) < iov.bufLen {
			break // short write
		}
	}
	*nwritten = total
	return 0
}

// channelReadv reads into each iovec through the channel using the given kernel
// syscall number (read or pread). offset < 0 selects the non-positioned form.
//
//go:nosplit
func channelReadv(number int32, fd int32, iovs *iovec, iovsLen size, offset int64, nread *size) Errno {
	var total size
	for i := size(0); i < iovsLen; i++ {
		iov := iovecAt(iovs, i)
		if iov.bufLen == 0 {
			continue
		}
		var ret int64
		var errno int32
		if offset < 0 {
			ret, errno = kandeloSyscall6(number, int64(fd), int64(iov.buf), int64(iov.bufLen), 0, 0, 0)
		} else {
			ret, errno = kandeloSyscall6(number, int64(fd), int64(iov.buf), int64(iov.bufLen), offset, 0, 0)
		}
		if errno != 0 {
			if total != 0 {
				break
			}
			return kandeloErrno(errno)
		}
		total += size(ret)
		if offset >= 0 {
			offset += ret
		}
		if size(ret) < iov.bufLen {
			break // short read / EOF
		}
	}
	*nread = total
	return 0
}

// fd_write gathers the iovec array through kernel write(2).
func fd_write(fd int32, iovs *iovec, iovsLen size, nwritten *size) Errno {
	return channelWritev(kSysWrite, fd, iovs, iovsLen, -1, nwritten)
}

func fd_sync(fd int32) Errno {
	_, errno := kandeloSyscall6(kSysFsync, int64(fd), 0, 0, 0, 0, 0)
	return kandeloErrno(errno)
}

func path_create_directory(fd int32, path *byte, pathLen size) Errno { return ENOSYS }

func path_filestat_get(fd int32, flags lookupflags, path *byte, pathLen size, buf unsafe.Pointer) Errno {
	return ENOSYS
}

func path_filestat_set_times(fd int32, flags lookupflags, path *byte, pathLen size, atim timestamp, mtim timestamp, fstflags fstflags) Errno {
	return ENOSYS
}

func path_link(oldFd int32, oldFlags lookupflags, oldPath *byte, oldPathLen size, newFd int32, newPath *byte, newPathLen size) Errno {
	return ENOSYS
}

func path_readlink(fd int32, path *byte, pathLen size, buf *byte, bufLen size, nwritten *size) Errno {
	return ENOSYS
}

func path_remove_directory(fd int32, path *byte, pathLen size) Errno { return ENOSYS }

func path_rename(oldFd int32, oldPath *byte, oldPathLen size, newFd int32, newPath *byte, newPathLen size) Errno {
	return ENOSYS
}

func path_symlink(oldPath *byte, oldPathLen size, fd int32, newPath *byte, newPathLen size) Errno {
	return ENOSYS
}

func path_unlink_file(fd int32, path *byte, pathLen size) Errno { return ENOSYS }

func path_open(rootFD int32, dirflags lookupflags, path *byte, pathLen size, oflags oflags, fsRightsBase rights, fsRightsInheriting rights, fsFlags fdflags, fd *int32) Errno {
	return ENOSYS
}

func random_get(buf *byte, bufLen size) Errno {
	for filled := size(0); filled < bufLen; {
		ret, errno := kandeloSyscall6(kSysGetrandom,
			int64(uintptr(unsafe.Pointer(buf))+uintptr(filled)),
			int64(bufLen-filled), 0, 0, 0, 0)
		if errno != 0 {
			if kandeloErrno(errno) == EINTR {
				continue
			}
			return kandeloErrno(errno)
		}
		if ret <= 0 || ret > int64(bufLen-filled) {
			return EIO
		}
		filled += size(ret)
	}
	return 0
}

// https://github.com/WebAssembly/WASI/blob/a2b96e81c0586125cc4dc79a5be0b78d9a059925/legacy/preview1/docs.md#-fdstat-record
// fdflags must be at offset 2, hence the uint16 type rather than the
// fdflags (uint32) type.
type fdstat struct {
	_                structs.HostLayout
	filetype         filetype
	fdflags          uint16
	rightsBase       rights
	rightsInheriting rights
}

// fd_fdstat_get synthesizes a WASI-style fdstat from the kernel. Kandelo has no
// single fdstat syscall, so the file type comes from fstat(2) and the flags are
// reported as zero (the channel is synchronous/blocking). Returning success
// keeps the os package's stdio setup and file-type queries working.
func fd_fdstat_get(fd int32, buf *fdstat) Errno {
	var st Stat_t
	if errno := fd_filestat_get(fd, unsafe.Pointer(&st)); errno != 0 {
		return errno
	}
	buf.filetype = st.Filetype
	buf.fdflags = 0
	buf.rightsBase = fullRights
	buf.rightsInheriting = fullRights
	return 0
}

func fd_fdstat_set_flags(fd int32, flags fdflags) Errno { return ENOSYS }

// fd_fdstat_get_flags is accessed from internal/syscall/unix
//go:linkname fd_fdstat_get_flags

func fd_fdstat_get_flags(fd int) (uint32, error) {
	var stat fdstat
	errno := fd_fdstat_get(int32(fd), &stat)
	return uint32(stat.fdflags), errnoErr(errno)
}

// fd_fdstat_get_type is accessed from net
//go:linkname fd_fdstat_get_type

func fd_fdstat_get_type(fd int) (uint8, error) {
	var stat fdstat
	errno := fd_fdstat_get(int32(fd), &stat)
	return stat.filetype, errnoErr(errno)
}

type preopentype = uint8

const (
	preopentypeDir preopentype = iota
)

type prestatDir struct {
	_         structs.HostLayout
	prNameLen size
}

type prestat struct {
	_   structs.HostLayout
	typ preopentype
	dir prestatDir
}

func fd_prestat_get(fd int32, prestat *prestat) Errno { return ENOSYS }

func fd_prestat_dir_name(fd int32, path *byte, pathLen size) Errno { return ENOSYS }

type opendir struct {
	fd   int32
	name string
}

// List of preopen directories that were exposed by the runtime. The first one
// is assumed to the be root directory of the file system, and others are seen
// as mount points at sub paths of the root.
var preopens []opendir

// Current working directory. We maintain this as a string and resolve paths in
// the code because wasmtime does not allow relative path lookups outside of the
// scope of a directory; a previous approach we tried consisted in maintaining
// open a file descriptor to the current directory so we could perform relative
// path lookups from that location, but it resulted in breaking path resolution
// from the current directory to its parent.
var cwd string

func init() {
	// Unlike wasip1, Kandelo does not expose a WASI preopen table: the kernel
	// resolves absolute and cwd-relative paths directly, so there is no fd
	// scan to perform here. We only seed the working-directory string used by
	// Getwd; the kernel is the authority for actual path resolution.
	if cwd, _ = Getenv("PWD"); cwd != "" {
		cwd = joinPath("/", cwd)
	} else {
		cwd = "/"
	}
}

// Provided by package runtime.
func now() (sec int64, nsec int32)

//go:nosplit
func appendCleanPath(buf []byte, path string, lookupParent bool) ([]byte, bool) {
	i := 0
	for i < len(path) {
		for i < len(path) && path[i] == '/' {
			i++
		}

		j := i
		for j < len(path) && path[j] != '/' {
			j++
		}

		s := path[i:j]
		i = j

		switch s {
		case "":
			continue
		case ".":
			continue
		case "..":
			if !lookupParent {
				k := len(buf)
				for k > 0 && buf[k-1] != '/' {
					k--
				}
				for k > 1 && buf[k-1] == '/' {
					k--
				}
				buf = buf[:k]
				if k == 0 {
					lookupParent = true
				} else {
					s = ""
					continue
				}
			}
		default:
			lookupParent = false
		}

		if len(buf) > 0 && buf[len(buf)-1] != '/' {
			buf = append(buf, '/')
		}
		buf = append(buf, s...)
	}
	return buf, lookupParent
}

// joinPath concatenates dir and file paths, producing a cleaned path where
// "." and ".." have been removed, unless dir is relative and the references
// to parent directories in file represented a location relative to a parent
// of dir.
//
// This function is used for path resolution of all wasi functions expecting
// a path argument; the returned string is heap allocated, which we may want
// to optimize in the future. Instead of returning a string, the function
// could append the result to an output buffer that the functions in this
// file can manage to have allocated on the stack (e.g. initializing to a
// fixed capacity). Since it will significantly increase code complexity,
// we prefer to optimize for readability and maintainability at this time.
func joinPath(dir, file string) string {
	buf := make([]byte, 0, len(dir)+len(file)+1)
	if isAbs(dir) {
		buf = append(buf, '/')
	}
	buf, lookupParent := appendCleanPath(buf, dir, false)
	buf, _ = appendCleanPath(buf, file, lookupParent)
	// The appendCleanPath function cleans the path so it does not inject
	// references to the current directory. If both the dir and file args
	// were ".", this results in the output buffer being empty so we handle
	// this condition here.
	if len(buf) == 0 {
		buf = append(buf, '.')
	}
	// If the file ended with a '/' we make sure that the output also ends
	// with a '/'. This is needed to ensure that programs have a mechanism
	// to represent dereferencing symbolic links pointing to directories.
	if buf[len(buf)-1] != '/' && isDir(file) {
		buf = append(buf, '/')
	}
	return unsafe.String(&buf[0], len(buf))
}

func isAbs(path string) bool {
	return stringslite.HasPrefix(path, "/")
}

func isDir(path string) bool {
	return stringslite.HasSuffix(path, "/")
}

// preparePath returns the preopen file descriptor of the directory to perform
// path resolution from, along with the pair of pointer and length for the
// relative expression of path from the directory.
//
// If the path argument is not absolute, it is first appended to the current
// working directory before resolution.
func preparePath(path string) (int32, *byte, size) {
	var dirFd = int32(-1)
	var dirName string

	dir := "/"
	if !isAbs(path) {
		dir = cwd
	}
	path = joinPath(dir, path)

	for _, p := range preopens {
		if len(p.name) > len(dirName) && stringslite.HasPrefix(path, p.name) {
			dirFd, dirName = p.fd, p.name
		}
	}

	path = path[len(dirName):]
	for isAbs(path) {
		path = path[1:]
	}
	if len(path) == 0 {
		path = "."
	}

	return dirFd, unsafe.StringData(path), size(len(path))
}

// Open opens path relative to the process working directory. Unlike the wasip1
// port, Kandelo has no WASI preopen table: the kernel resolves ordinary
// absolute and cwd-relative paths directly, so Open routes straight to the
// kernel openat(2) with AT_FDCWD and Linux-style open flags (the O_* constants
// in syscall_kandelo.go already carry Linux values).
func Open(path string, openmode int, perm uint32) (int, error) {
	return Openat(int(kAtFdcwd), path, openmode, perm)
}

func Openat(dirFd int, path string, openmode int, perm uint32) (int, error) {
	if path == "" {
		return -1, ENOENT
	}
	p, err := BytePtrFromString(path)
	if err != nil {
		return -1, err
	}
	ret, errno := kandeloSyscall6(kSysOpenat, int64(dirFd),
		int64(uintptr(unsafe.Pointer(p))), int64(openmode), int64(perm), 0, 0)
	runtime.KeepAlive(p)
	if errno != 0 {
		return -1, kandeloErrno(errno)
	}
	return int(ret), nil
}

func Close(fd int) error {
	errno := fd_close(int32(fd))
	return errnoErr(errno)
}

func CloseOnExec(fd int) {
	// nothing to do - no exec
}

// pathArg converts a Go string into a NUL-terminated pointer suitable for a
// channel path argument (the kernel derives the length from the terminator, as
// it does for openat). The returned *byte must be kept alive across the syscall
// with runtime.KeepAlive.
func pathArg(path string) (*byte, int64, error) {
	p, err := BytePtrFromString(path)
	if err != nil {
		return nil, 0, err
	}
	return p, int64(uintptr(unsafe.Pointer(p))), nil
}

func Mkdir(path string, perm uint32) error {
	return Mkdirat(int(kAtFdcwd), path, perm)
}

// Mkdirat creates a directory relative to dirFd via the kernel mkdirat(2).
// Kandelo resolves cwd-relative paths in the kernel, so callers pass ordinary
// absolute or cwd-relative paths with kAtFdcwd, matching Openat.
func Mkdirat(dirFd int, path string, perm uint32) error {
	if path == "" {
		return ENOENT
	}
	p, ptr, err := pathArg(path)
	if err != nil {
		return err
	}
	_, errno := kandeloSyscall6(kSysMkdirat, int64(dirFd), ptr, int64(perm), 0, 0, 0)
	runtime.KeepAlive(p)
	return errnoErr(kandeloErrno(errno))
}

func ReadDir(fd int, buf []byte, cookie dircookie) (int, error) {
	var nwritten size
	errno := fd_readdir(int32(fd), &buf[0], size(len(buf)), cookie, &nwritten)
	return int(nwritten), errnoErr(errno)
}

type Stat_t struct {
	Dev      uint64
	Ino      uint64
	Filetype uint8
	Nlink    uint64
	Size     uint64
	Atime    uint64
	Mtime    uint64
	Ctime    uint64

	Mode int

	// Uid and Gid are always zero on kandelo platforms
	Uid uint32
	Gid uint32
}

// aTSymlinkNofollow matches Linux AT_SYMLINK_NOFOLLOW; Lstat passes it to
// fstatat so symbolic links are not dereferenced.
const aTSymlinkNofollow = 0x100

func channelStatAt(dirFd int32, path string, flags int, st *Stat_t) error {
	if path == "" {
		return ENOENT
	}
	p, err := BytePtrFromString(path)
	if err != nil {
		return err
	}
	var ks kernelStat
	_, errno := kandeloSyscall6(kSysFstatat, int64(dirFd),
		int64(uintptr(unsafe.Pointer(p))), int64(uintptr(unsafe.Pointer(&ks))),
		int64(flags), 0, 0)
	runtime.KeepAlive(p)
	if errno != 0 {
		return kandeloErrno(errno)
	}
	ks.toStat(st)
	return nil
}

func Stat(path string, st *Stat_t) error {
	return channelStatAt(kAtFdcwd, path, 0, st)
}

func Lstat(path string, st *Stat_t) error {
	return channelStatAt(kAtFdcwd, path, aTSymlinkNofollow, st)
}

func Fstat(fd int, st *Stat_t) error {
	errno := fd_filestat_get(int32(fd), unsafe.Pointer(st))
	return errnoErr(errno)
}

func setDefaultMode(st *Stat_t) {
	// WASI does not support unix-like permissions, but Go programs are likely
	// to expect the permission bits to not be zero so we set defaults to help
	// avoid breaking applications that are migrating to WASM.
	if st.Filetype == FILETYPE_DIRECTORY {
		st.Mode = 0700
	} else {
		st.Mode = 0600
	}
}

func Unlink(path string) error {
	return Unlinkat(int(kAtFdcwd), path, 0)
}

// Unlinkat removes a name relative to dirFd via the kernel unlinkat(2). The
// AT_REMOVEDIR flag (kAtRemovedir) selects directory removal, which is how
// Rmdir is expressed on this arch.
func Unlinkat(dirFd int, path string, flags int) error {
	if path == "" {
		return ENOENT
	}
	p, ptr, err := pathArg(path)
	if err != nil {
		return err
	}
	_, errno := kandeloSyscall6(kSysUnlinkat, int64(dirFd), ptr, int64(flags), 0, 0, 0)
	runtime.KeepAlive(p)
	return errnoErr(kandeloErrno(errno))
}

func Rmdir(path string) error {
	return Unlinkat(int(kAtFdcwd), path, int(kAtRemovedir))
}

func Chmod(path string, mode uint32) error {
	if path == "" {
		return ENOENT
	}
	p, ptr, err := pathArg(path)
	if err != nil {
		return err
	}
	_, errno := kandeloSyscall6(kSysChmod, ptr, int64(mode), 0, 0, 0, 0)
	runtime.KeepAlive(p)
	return errnoErr(kandeloErrno(errno))
}

func Fchmod(fd int, mode uint32) error {
	_, errno := kandeloSyscall6(kSysFchmod, int64(fd), int64(mode), 0, 0, 0, 0)
	return errnoErr(kandeloErrno(errno))
}

// Fchmodat changes a file mode relative to dirFd via the kernel fchmodat(2).
func Fchmodat(dirFd int, path string, mode uint32, flags int) error {
	if path == "" {
		return ENOENT
	}
	p, ptr, err := pathArg(path)
	if err != nil {
		return err
	}
	_, errno := kandeloSyscall6(kSysFchmodat, int64(dirFd), ptr, int64(mode), int64(flags), 0, 0)
	runtime.KeepAlive(p)
	return errnoErr(kandeloErrno(errno))
}

func Chown(path string, uid, gid int) error {
	return ENOSYS
}

func Fchown(fd int, uid, gid int) error {
	return ENOSYS
}

func Lchown(path string, uid, gid int) error {
	return ENOSYS
}

// Go's UTIME_OMIT sentinel (matches internal/syscall/unix and os _UTIME_OMIT).
const goUTIME_OMIT = -0x2

// Kernel utimensat tv_nsec sentinels (crates/runtime-core/src/syscalls.rs).
// The kernel interprets these directly in the WasmTimespec it reads, so the
// Go-side sentinel must be translated before the channel call.
const (
	kUtimeNow  = 0x3fffffff
	kUtimeOmit = 0x3ffffffe
)

// Utimensat sets the access/modification times of path (relative to dirFd) via
// the kernel utimensat(2). Go's UTIME_OMIT sentinel is translated to the
// kernel's tv_nsec sentinel so an omitted timestamp is left unchanged by the
// kernel rather than requiring a pre-stat here.
func Utimensat(dirFd int, path string, ts *[2]Timespec, flags int) error {
	// Timespec has the same {Sec, Nsec int64} layout as the kernel's
	// WasmTimespec, so a translated copy can be passed by pointer directly.
	var kt [2]Timespec
	for i := 0; i < 2; i++ {
		kt[i] = ts[i]
		if ts[i].Nsec == goUTIME_OMIT {
			kt[i].Nsec = kUtimeOmit
		}
	}

	var p *byte
	var pathPtr int64
	if path != "" {
		var err error
		p, pathPtr, err = pathArg(path)
		if err != nil {
			return err
		}
	}
	_, errno := kandeloSyscall6(kSysUtimensat, int64(dirFd), pathPtr,
		int64(uintptr(unsafe.Pointer(&kt[0]))), int64(flags), 0, 0)
	runtime.KeepAlive(p)
	runtime.KeepAlive(&kt)
	return errnoErr(kandeloErrno(errno))
}

func UtimesNano(path string, ts []Timespec) error {
	if path == "" {
		return ENOENT
	}
	if len(ts) != 2 {
		return EINVAL
	}
	var a [2]Timespec
	a[0], a[1] = ts[0], ts[1]
	return Utimensat(int(kAtFdcwd), path, &a, 0)
}

func Rename(from, to string) error {
	return Renameat(int(kAtFdcwd), from, int(kAtFdcwd), to)
}

// Renameat renames oldpath (relative to oldDirFd) to newpath (relative to
// newDirFd) via the kernel renameat(2).
func Renameat(oldDirFd int, oldpath string, newDirFd int, newpath string) error {
	if oldpath == "" || newpath == "" {
		return ENOENT
	}
	oldP, oldPtr, err := pathArg(oldpath)
	if err != nil {
		return err
	}
	newP, newPtr, err := pathArg(newpath)
	if err != nil {
		return err
	}
	_, errno := kandeloSyscall6(kSysRenameat, int64(oldDirFd), oldPtr, int64(newDirFd), newPtr, 0, 0)
	runtime.KeepAlive(oldP)
	runtime.KeepAlive(newP)
	return errnoErr(kandeloErrno(errno))
}

// Fstatat fills st from the kernel fstatat(2) for path relative to dirFd. flags
// carries AT_SYMLINK_NOFOLLOW (aTSymlinkNofollow) for Lstat-style lookups.
func Fstatat(dirFd int, path string, st *Stat_t, flags int) error {
	return channelStatAt(int32(dirFd), path, flags, st)
}

func Truncate(path string, length int64) error {
	if path == "" {
		return ENOENT
	}
	p, ptr, err := pathArg(path)
	if err != nil {
		return err
	}
	_, errno := kandeloSyscall6(kSysTruncate, ptr, length, 0, 0, 0, 0)
	runtime.KeepAlive(p)
	return errnoErr(kandeloErrno(errno))
}

func Ftruncate(fd int, length int64) error {
	errno := fd_filestat_set_size(int32(fd), filesize(length))
	return errnoErr(errno)
}

const ImplementsGetwd = true

// Getwd returns the process working directory from the kernel getcwd(2). The
// kernel is authoritative for path resolution, so this asks it directly rather
// than trusting the cached cwd string. The kernel writes the path with a
// trailing NUL and returns the length including that NUL (Linux convention).
func Getwd() (string, error) {
	for size := 128; size <= 1<<16; size *= 2 {
		buf := make([]byte, size)
		ret, errno := kandeloSyscall6(kSysGetcwd,
			int64(uintptr(unsafe.Pointer(&buf[0]))), int64(size), 0, 0, 0, 0)
		if errno == 0 {
			n := int(ret)
			if n > 0 && buf[n-1] == 0 {
				n-- // drop the trailing NUL the kernel included in the count
			}
			cwd = string(buf[:n])
			return cwd, nil
		}
		if kandeloErrno(errno) != ERANGE {
			return "", kandeloErrno(errno)
		}
	}
	return "", ERANGE
}

func Chdir(path string) error {
	if path == "" {
		return ENOENT
	}
	p, ptr, err := pathArg(path)
	if err != nil {
		return err
	}
	_, errno := kandeloSyscall6(kSysChdir, ptr, 0, 0, 0, 0, 0)
	runtime.KeepAlive(p)
	if errno != 0 {
		return kandeloErrno(errno)
	}
	// Keep the cached cwd string coherent for any residual local resolver use.
	if isAbs(path) {
		cwd = joinPath("/", path)
	} else {
		cwd = joinPath(cwd, path)
	}
	return nil
}

// Fchdir changes the working directory to the one referenced by fd via the
// kernel fchdir(2). The cached cwd string is refreshed from the kernel so
// later Getwd/relative resolution stays coherent.
func Fchdir(fd int) error {
	_, errno := kandeloSyscall6(kSysFchdir, int64(fd), 0, 0, 0, 0, 0)
	if errno != 0 {
		return kandeloErrno(errno)
	}
	if wd, err := Getwd(); err == nil {
		cwd = wd
	}
	return nil
}

func Readlink(path string, buf []byte) (n int, err error) {
	return Readlinkat(int(kAtFdcwd), path, buf)
}

// Readlinkat reads the target of the symlink at path (relative to dirFd) into
// buf via the kernel readlinkat(2), returning the number of bytes written.
func Readlinkat(dirFd int, path string, buf []byte) (int, error) {
	if path == "" {
		return 0, ENOENT
	}
	if len(buf) == 0 {
		return 0, nil
	}
	p, ptr, err := pathArg(path)
	if err != nil {
		return 0, err
	}
	ret, errno := kandeloSyscall6(kSysReadlinkat, int64(dirFd), ptr,
		int64(uintptr(unsafe.Pointer(&buf[0]))), int64(len(buf)), 0, 0)
	runtime.KeepAlive(p)
	runtime.KeepAlive(buf)
	if errno != 0 {
		return 0, kandeloErrno(errno)
	}
	return int(ret), nil
}

func Link(path, link string) error {
	return Linkat(int(kAtFdcwd), path, int(kAtFdcwd), link, 0)
}

// Linkat creates newpath (relative to newDirFd) as a hard link to oldpath
// (relative to oldDirFd) via the kernel linkat(2).
func Linkat(oldDirFd int, oldpath string, newDirFd int, newpath string, flags int) error {
	if oldpath == "" || newpath == "" {
		return ENOENT
	}
	oldP, oldPtr, err := pathArg(oldpath)
	if err != nil {
		return err
	}
	newP, newPtr, err := pathArg(newpath)
	if err != nil {
		return err
	}
	_, errno := kandeloSyscall6(kSysLinkat, int64(oldDirFd), oldPtr, int64(newDirFd), newPtr, int64(flags), 0)
	runtime.KeepAlive(oldP)
	runtime.KeepAlive(newP)
	return errnoErr(kandeloErrno(errno))
}

func Symlink(path, link string) error {
	return Symlinkat(path, int(kAtFdcwd), link)
}

// Symlinkat creates the symlink linkpath (relative to newDirFd) pointing at
// target via the kernel symlinkat(2). target is stored verbatim and is not
// resolved, so it is passed as an ordinary NUL-terminated string.
func Symlinkat(target string, newDirFd int, linkpath string) error {
	if target == "" || linkpath == "" {
		return ENOENT
	}
	tP, tPtr, err := pathArg(target)
	if err != nil {
		return err
	}
	lP, lPtr, err := pathArg(linkpath)
	if err != nil {
		return err
	}
	_, errno := kandeloSyscall6(kSysSymlinkat, tPtr, int64(newDirFd), lPtr, 0, 0, 0)
	runtime.KeepAlive(tP)
	runtime.KeepAlive(lP)
	return errnoErr(kandeloErrno(errno))
}

func Fsync(fd int) error {
	errno := fd_sync(int32(fd))
	return errnoErr(errno)
}

func makeIOVec(b []byte) *iovec {
	return &iovec{
		buf:    uintptr32(uintptr(unsafe.Pointer(unsafe.SliceData(b)))),
		bufLen: size(len(b)),
	}
}

func Read(fd int, b []byte) (int, error) {
	var nread size
	errno := fd_read(int32(fd), makeIOVec(b), 1, &nread)
	runtime.KeepAlive(b)
	return int(nread), errnoErr(errno)
}

func Write(fd int, b []byte) (int, error) {
	var nwritten size
	errno := fd_write(int32(fd), makeIOVec(b), 1, &nwritten)
	runtime.KeepAlive(b)
	return int(nwritten), errnoErr(errno)
}

func Pread(fd int, b []byte, offset int64) (int, error) {
	var nread size
	errno := fd_pread(int32(fd), makeIOVec(b), 1, filesize(offset), &nread)
	runtime.KeepAlive(b)
	return int(nread), errnoErr(errno)
}

func Pwrite(fd int, b []byte, offset int64) (int, error) {
	var nwritten size
	errno := fd_pwrite(int32(fd), makeIOVec(b), 1, filesize(offset), &nwritten)
	runtime.KeepAlive(b)
	return int(nwritten), errnoErr(errno)
}

func Seek(fd int, offset int64, whence int) (int64, error) {
	var newoffset filesize
	errno := fd_seek(int32(fd), filedelta(offset), uint32(whence), &newoffset)
	return int64(newoffset), errnoErr(errno)
}

func Dup(fd int) (int, error) {
	return 0, ENOSYS
}

func Dup2(fd, newfd int) error {
	return ENOSYS
}

func Pipe(fd []int) error {
	return ENOSYS
}

func RandomGet(b []byte) error {
	errno := random_get(unsafe.SliceData(b), size(len(b)))
	return errnoErr(errno)
}

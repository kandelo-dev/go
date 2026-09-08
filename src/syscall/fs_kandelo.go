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
	if bufLen == 0 {
		return 0
	}
	_, errno := kandeloSyscall6(kSysGetrandom, int64(uintptr(unsafe.Pointer(buf))), int64(bufLen), 0, 0, 0, 0)
	return kandeloErrno(errno)
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

// fd_fdstat_set_flags is a no-op success. The Kandelo syscall channel is
// synchronous and blocking, so non-blocking flag changes have no backing
// mechanism; reporting success lets os.NewFile proceed without treating stdio
// setup as a hard failure.
func fd_fdstat_set_flags(fd int32, flags fdflags) Errno { return 0 }

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

func Mkdir(path string, perm uint32) error {
	if path == "" {
		return EINVAL
	}
	dirFd, pathPtr, pathLen := preparePath(path)
	errno := path_create_directory(dirFd, pathPtr, pathLen)
	return errnoErr(errno)
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
	if path == "" {
		return EINVAL
	}
	dirFd, pathPtr, pathLen := preparePath(path)
	errno := path_unlink_file(dirFd, pathPtr, pathLen)
	return errnoErr(errno)
}

func Rmdir(path string) error {
	if path == "" {
		return EINVAL
	}
	dirFd, pathPtr, pathLen := preparePath(path)
	errno := path_remove_directory(dirFd, pathPtr, pathLen)
	return errnoErr(errno)
}

func Chmod(path string, mode uint32) error {
	var stat Stat_t
	return Stat(path, &stat)
}

func Fchmod(fd int, mode uint32) error {
	var stat Stat_t
	return Fstat(fd, &stat)
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

func UtimesNano(path string, ts []Timespec) error {
	// UTIME_OMIT value must match internal/syscall/unix/at_wasip1.go
	const UTIME_OMIT = -0x2
	if path == "" {
		return EINVAL
	}
	dirFd, pathPtr, pathLen := preparePath(path)
	atime := TimespecToNsec(ts[0])
	mtime := TimespecToNsec(ts[1])
	if ts[0].Nsec == UTIME_OMIT || ts[1].Nsec == UTIME_OMIT {
		var st Stat_t
		if err := Stat(path, &st); err != nil {
			return err
		}
		if ts[0].Nsec == UTIME_OMIT {
			atime = int64(st.Atime)
		}
		if ts[1].Nsec == UTIME_OMIT {
			mtime = int64(st.Mtime)
		}
	}
	errno := path_filestat_set_times(
		dirFd,
		LOOKUP_SYMLINK_FOLLOW,
		pathPtr,
		pathLen,
		timestamp(atime),
		timestamp(mtime),
		FILESTAT_SET_ATIM|FILESTAT_SET_MTIM,
	)
	return errnoErr(errno)
}

func Rename(from, to string) error {
	if from == "" || to == "" {
		return EINVAL
	}
	oldDirFd, oldPathPtr, oldPathLen := preparePath(from)
	newDirFd, newPathPtr, newPathLen := preparePath(to)
	errno := path_rename(
		oldDirFd,
		oldPathPtr,
		oldPathLen,
		newDirFd,
		newPathPtr,
		newPathLen,
	)
	return errnoErr(errno)
}

func Truncate(path string, length int64) error {
	if path == "" {
		return EINVAL
	}
	fd, err := Open(path, O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer Close(fd)
	return Ftruncate(fd, length)
}

func Ftruncate(fd int, length int64) error {
	errno := fd_filestat_set_size(int32(fd), filesize(length))
	return errnoErr(errno)
}

const ImplementsGetwd = true

func Getwd() (string, error) {
	return cwd, nil
}

func Chdir(path string) error {
	if path == "" {
		return EINVAL
	}

	dir := "/"
	if !isAbs(path) {
		dir = cwd
	}
	path = joinPath(dir, path)

	var stat Stat_t
	dirFd, pathPtr, pathLen := preparePath(path)
	errno := path_filestat_get(dirFd, LOOKUP_SYMLINK_FOLLOW, pathPtr, pathLen, unsafe.Pointer(&stat))
	if errno != 0 {
		return errnoErr(errno)
	}
	if stat.Filetype != FILETYPE_DIRECTORY {
		return ENOTDIR
	}
	cwd = path
	return nil
}

func Readlink(path string, buf []byte) (n int, err error) {
	if path == "" {
		return 0, EINVAL
	}
	if len(buf) == 0 {
		return 0, nil
	}
	dirFd, pathPtr, pathLen := preparePath(path)
	var nwritten size
	errno := path_readlink(
		dirFd,
		pathPtr,
		pathLen,
		&buf[0],
		size(len(buf)),
		&nwritten,
	)
	// For some reason wasmtime returns ERANGE when the output buffer is
	// shorter than the symbolic link value. os.Readlink expects a nil
	// error and uses the fact that n is greater or equal to the buffer
	// length to assume that it needs to try again with a larger size.
	// This condition is handled in os.Readlink.
	return int(nwritten), errnoErr(errno)
}

func Link(path, link string) error {
	if path == "" || link == "" {
		return EINVAL
	}
	oldDirFd, oldPathPtr, oldPathLen := preparePath(path)
	newDirFd, newPathPtr, newPathLen := preparePath(link)
	errno := path_link(
		oldDirFd,
		0,
		oldPathPtr,
		oldPathLen,
		newDirFd,
		newPathPtr,
		newPathLen,
	)
	return errnoErr(errno)
}

func Symlink(path, link string) error {
	if path == "" || link == "" {
		return EINVAL
	}
	dirFd, pathPtr, pathlen := preparePath(link)
	errno := path_symlink(
		unsafe.StringData(path),
		size(len(path)),
		dirFd,
		pathPtr,
		pathlen,
	)
	return errnoErr(errno)
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

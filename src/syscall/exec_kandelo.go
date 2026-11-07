//go:build kandelo

package syscall

import (
	"encoding/binary"
	"runtime"
	"strings"
	"unsafe"
)

const (
	spawnHeaderBytes = 40
	spawnActionBytes = 28
	spawnArgMax      = 4 * 1024 * 1024
	spawnPathMax     = 4096
	spawnMaxStrings  = 4096
	spawnMaxActions  = 1024
	spawnOpClose     = 1
	spawnOpDup2      = 2
	spawnOpChdir     = 3
)

func StartProcess(argv0 string, argv []string, attr *ProcAttr) (pid int, handle uintptr, err error) {
	if attr == nil {
		attr = &ProcAttr{}
	}
	if argv0 == "" {
		return 0, 0, ENOENT
	}
	if len(argv0) >= spawnPathMax {
		return 0, 0, ENAMETOOLONG
	}
	if strings.IndexByte(argv0, 0) >= 0 {
		return 0, 0, EINVAL
	}
	if len(argv) > spawnMaxStrings || len(attr.Env) > spawnMaxStrings {
		return 0, 0, E2BIG
	}
	if attr.Dir != "" && len(attr.Dir) >= spawnPathMax {
		return 0, 0, ENAMETOOLONG
	}
	if strings.IndexByte(attr.Dir, 0) >= 0 {
		return 0, 0, EINVAL
	}
	if attr.Sys != nil {
		if attr.Sys.Credential != nil {
			return 0, 0, ENOSYS
		}
		if attr.Sys.Setpgid && (attr.Sys.Pgid < 0 || attr.Sys.Pgid > 0x7fffffff) {
			return 0, 0, EINVAL
		}
	}

	stringBytes := 0
	for _, value := range argv {
		if strings.IndexByte(value, 0) >= 0 {
			return 0, 0, EINVAL
		}
		if len(value) > 65535 {
			return 0, 0, E2BIG
		}
		stringBytes += len(value) + 1
	}
	for _, value := range attr.Env {
		if strings.IndexByte(value, 0) >= 0 {
			return 0, 0, EINVAL
		}
		if len(value) > 65535 {
			return 0, 0, E2BIG
		}
		stringBytes += len(value) + 1
	}
	if stringBytes+4*(len(argv)+len(attr.Env)+2) > spawnArgMax {
		return 0, 0, E2BIG
	}

	actionCount := len(attr.Files)
	pathBytes := 0
	if attr.Dir != "" {
		actionCount++
		pathBytes = len(attr.Dir) + 1
	}
	if actionCount > spawnMaxActions {
		return 0, 0, E2BIG
	}
	blob := make([]byte, spawnHeaderBytes+4*(len(argv)+len(attr.Env))+spawnActionBytes*actionCount+stringBytes+pathBytes)
	binary.LittleEndian.PutUint32(blob[0:], uint32(len(argv)))
	binary.LittleEndian.PutUint32(blob[4:], uint32(len(attr.Env)))
	binary.LittleEndian.PutUint32(blob[8:], uint32(actionCount))
	if attr.Sys != nil && attr.Sys.Setpgid {
		binary.LittleEndian.PutUint32(blob[12:], 0x02)
		binary.LittleEndian.PutUint32(blob[16:], uint32(attr.Sys.Pgid))
	}
	actionsBase := spawnHeaderBytes + 4*(len(argv)+len(attr.Env))
	stringsBase := actionsBase + spawnActionBytes*actionCount
	cursor := 0
	for index, value := range argv {
		binary.LittleEndian.PutUint32(blob[spawnHeaderBytes+4*index:], uint32(cursor))
		copy(blob[stringsBase+cursor:], value)
		cursor += len(value) + 1
	}
	for index, value := range attr.Env {
		binary.LittleEndian.PutUint32(blob[spawnHeaderBytes+4*(len(argv)+index):], uint32(cursor))
		copy(blob[stringsBase+cursor:], value)
		cursor += len(value) + 1
	}
	actionIndex := 0
	if attr.Dir != "" {
		record := blob[actionsBase:]
		binary.LittleEndian.PutUint32(record[0:], spawnOpChdir)
		binary.LittleEndian.PutUint32(record[12:], uint32(cursor))
		binary.LittleEndian.PutUint32(record[16:], uint32(len(attr.Dir)+1))
		copy(blob[stringsBase+cursor:], attr.Dir)
		actionIndex++
	}
	ForkLock.Lock()
	defer ForkLock.Unlock()
	temporary := make([]int, 0, len(attr.Files))
	defer func() {
		for _, fd := range temporary {
			Close(fd)
		}
	}()
	for target, source := range attr.Files {
		record := blob[actionsBase+actionIndex*spawnActionBytes:]
		if source == ^uintptr(0) {
			binary.LittleEndian.PutUint32(record[0:], spawnOpClose)
			binary.LittleEndian.PutUint32(record[4:], uint32(target))
		} else {
			if source > 0x7fffffff {
				return 0, 0, EBADF
			}
			copyFd, copyErr := Fcntl(int(source), F_DUPFD_CLOEXEC, len(attr.Files))
			if copyErr != nil {
				return 0, 0, copyErr
			}
			temporary = append(temporary, copyFd)
			binary.LittleEndian.PutUint32(record[0:], spawnOpDup2)
			binary.LittleEndian.PutUint32(record[4:], uint32(copyFd))
			binary.LittleEndian.PutUint32(record[8:], uint32(target))
		}
		actionIndex++
	}
	var childPid int32
	ret, errno := kandeloSyscall6(kSysSpawn, int64(uintptr(unsafe.Pointer(unsafe.StringData(argv0)))), int64(len(argv0)), int64(uintptr(unsafe.Pointer(&blob[0]))), int64(len(blob)), int64(uintptr(unsafe.Pointer(&childPid))), 0)
	runtime.KeepAlive(argv0)
	runtime.KeepAlive(blob)
	runtime.KeepAlive(&childPid)
	if errno != 0 {
		return 0, 0, kandeloErrno(errno)
	}
	if ret != 0 {
		return 0, 0, EIO
	}
	return int(childPid), 0, nil
}

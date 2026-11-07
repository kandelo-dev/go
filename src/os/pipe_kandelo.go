//go:build kandelo

package os

import "syscall"

func Pipe() (reader *File, writer *File, err error) {
	var pair [2]int
	if err := syscall.Pipe2(pair[:], syscall.O_CLOEXEC); err != nil {
		return nil, nil, NewSyscallError("pipe2", err)
	}
	return newFile(pair[0], "|0", kindPipe, false), newFile(pair[1], "|1", kindPipe, false), nil
}

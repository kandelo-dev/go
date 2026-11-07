// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package poll

import (
	"sync/atomic"
	"syscall"
)

type SysFile struct {
	// RefCountPtr is a pointer to the reference count of Sysfd.
	//
	// WASI preview 1 lacks a dup(2) system call. When the os and net packages
	// need to share a file/socket, instead of duplicating the underlying file
	// descriptor, we instead provide a way to copy FD instances and manage the
	// underlying file descriptor with reference counting.
	RefCountPtr *int32

	// RefCount is the reference count of Sysfd. When a copy of an FD is made,
	// it points to the reference count of the original FD instance.
	RefCount int32

	// Absolute path of the file, as returned by syscall.PathOpen;
	// this is used by Fchdir to emulate setting the current directory
	// to an open file descriptor.
	Path string
}

func (s *SysFile) init() {
	if s.RefCountPtr == nil {
		s.RefCount = 1
		s.RefCountPtr = &s.RefCount
	}
}

func (s *SysFile) ref() SysFile {
	atomic.AddInt32(s.RefCountPtr, +1)
	return SysFile{RefCountPtr: s.RefCountPtr}
}

func (s *SysFile) destroy(fd int) error {
	if s.RefCountPtr != nil && atomic.AddInt32(s.RefCountPtr, -1) > 0 {
		return nil
	}

	// We don't use ignoringEINTR here because POSIX does not define
	// whether the descriptor is closed if close returns EINTR.
	// If the descriptor is indeed closed, using a loop would race
	// with some other goroutine opening a new descriptor.
	// (The Linux kernel guarantees that it is closed on an EINTR error.)
	return CloseFunc(fd)
}

// Copy creates a copy of the FD.
//
// The FD instance points to the same underlying file descriptor. The file
// descriptor isn't closed until all FD instances that refer to it have been
// closed/destroyed.
func (fd *FD) Copy() FD {
	return FD{
		Sysfd:         fd.Sysfd,
		SysFile:       fd.SysFile.ref(),
		IsStream:      fd.IsStream,
		ZeroReadIsEOF: fd.ZeroReadIsEOF,
		isBlocking:    fd.isBlocking,
		isFile:        fd.isFile,
	}
}

// dupCloseOnExecOld always errors on wasip1 because there is no mechanism to
// duplicate file descriptors.
func dupCloseOnExecOld(fd int) (int, string, error) {
	return -1, "dup", syscall.ENOSYS
}

// Fchdir wraps syscall.Fchdir.
func (fd *FD) Fchdir() error {
	if err := fd.incref(); err != nil {
		return err
	}
	defer fd.decref()
	return syscall.Chdir(fd.Path)
}

// ReadDir wraps syscall.ReadDir.
// We treat this like an ordinary system call rather than a call
// that tries to fill the buffer.
func (fd *FD) ReadDir(buf []byte, cookie syscall.Dircookie) (int, error) {
	if err := fd.incref(); err != nil {
		return 0, err
	}
	defer fd.decref()
	for {
		n, err := syscall.ReadDir(fd.Sysfd, buf, cookie)
		if err != nil {
			n = 0
			if err == syscall.EAGAIN && fd.pd.pollable() {
				if err = fd.pd.waitRead(fd.isFile); err == nil {
					continue
				}
			}
		}
		// Do not call eofError; caller does not expect to see io.EOF.
		return n, err
	}
}

func (fd *FD) ReadDirent(buf []byte) (int, error) {
	return fd.ReadDir(buf, 0)
}

// Seek wraps syscall.Seek.
func (fd *FD) Seek(offset int64, whence int) (int64, error) {
	if err := fd.incref(); err != nil {
		return 0, err
	}
	defer fd.decref()
	return syscall.Seek(fd.Sysfd, offset, whence)
}

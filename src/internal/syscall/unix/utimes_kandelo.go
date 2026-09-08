// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build kandelo

package unix

import "syscall"

// Utimensat sets file times relative to dirfd through the kernel utimensat(2)
// over the Kandelo channel (syscall.Utimensat), rather than the wasip1
// path_filestat_set_times import the Kandelo host does not provide.
func Utimensat(dirfd int, path string, times *[2]syscall.Timespec, flag int) error {
	return syscall.Utimensat(dirfd, path, times, flag)
}

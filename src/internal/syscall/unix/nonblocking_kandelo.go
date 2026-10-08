// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build kandelo

package unix

import "syscall"

func IsNonblock(fd int) (nonblocking bool, err error) {
	flags, err := syscall.Fcntl(fd, syscall.F_GETFL, 0)
	if err != nil {
		return false, err
	}
	return flags&syscall.O_NONBLOCK != 0, nil
}

func HasNonblockFlag(flag int) bool {
	return flag&syscall.O_NONBLOCK != 0
}

package sysrand

import "syscall"

func read(b []byte) error {
	return syscall.RandomGet(b)
}

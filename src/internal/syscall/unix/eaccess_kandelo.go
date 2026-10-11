//go:build kandelo

package unix

import "syscall"

func Eaccess(path string, mode uint32) error {
	return syscall.Faccessat(-100, path, mode, 0x200)
}

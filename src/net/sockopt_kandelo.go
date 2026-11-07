package net

import (
	"os"
	"syscall"
)

func setDefaultSockopts(socket, family, socketType int, ipv6only bool) error {
	if family == syscall.AF_INET6 && socketType != syscall.SOCK_RAW {
		if err := syscall.SetsockoptInt(socket, syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, boolint(ipv6only)); err != nil {
			return os.NewSyscallError("setsockopt", err)
		}
	}
	if (socketType == syscall.SOCK_DGRAM || socketType == syscall.SOCK_RAW) && family != syscall.AF_UNIX {
		return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(socket, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1))
	}
	return nil
}

func setDefaultListenerSockopts(socket int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(socket, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1))
}

func setDefaultMulticastSockopts(socket int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(socket, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1))
}

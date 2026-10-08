package net

import "syscall"

func interfaceTable(ifindex int) ([]Interface, error) {
	return nil, syscall.ENOSYS
}

func interfaceAddrTable(ifi *Interface) ([]Addr, error) {
	return nil, syscall.ENOSYS
}

func interfaceMulticastAddrTable(ifi *Interface) ([]Addr, error) {
	return nil, syscall.ENOSYS
}

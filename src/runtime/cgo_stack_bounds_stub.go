//go:build !kandelo || !cgo

package runtime

func kandeloCgoStackBounds(sp uintptr) (uintptr, uintptr) { return 0, 0 }

func kandeloCgoChannelBase() uint32 { return 0 }

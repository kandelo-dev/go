//go:build kandelo && cgo

package runtime

var cgoCStack [8 << 20]byte

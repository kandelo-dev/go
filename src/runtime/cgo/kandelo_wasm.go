//go:build kandelo && wasm

package cgo

import _ "unsafe"

//go:cgo_import_static x_cgo_kandelo_init
//go:linkname x_cgo_kandelo_init x_cgo_kandelo_init
var x_cgo_kandelo_init byte

//go:cgo_import_static x_cgo_kandelo_get_channel_base
//go:linkname x_cgo_kandelo_get_channel_base x_cgo_kandelo_get_channel_base
var x_cgo_kandelo_get_channel_base byte

//go:cgo_import_static x_cgo_kandelo_signal_setup
//go:linkname x_cgo_kandelo_signal_setup x_cgo_kandelo_signal_setup
var x_cgo_kandelo_signal_setup byte

//go:linkname _cgo_kandelo_signal_setup _cgo_kandelo_signal_setup
var _cgo_kandelo_signal_setup = &x_cgo_kandelo_signal_setup

//go:cgo_import_static x_cgo_kandelo_signal_dispatch
//go:linkname x_cgo_kandelo_signal_dispatch x_cgo_kandelo_signal_dispatch
var x_cgo_kandelo_signal_dispatch byte

//go:linkname _cgo_kandelo_signal_dispatch _cgo_kandelo_signal_dispatch
var _cgo_kandelo_signal_dispatch = &x_cgo_kandelo_signal_dispatch

//go:cgo_import_static x_cgo_kandelo_syscall
//go:linkname x_cgo_kandelo_syscall x_cgo_kandelo_syscall
var x_cgo_kandelo_syscall byte

//go:linkname _cgo_kandelo_syscall _cgo_kandelo_syscall
var _cgo_kandelo_syscall = &x_cgo_kandelo_syscall

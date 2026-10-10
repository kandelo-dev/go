//go:build kandelo && wasm

package cgo

import _ "unsafe"

//go:cgo_import_static x_cgo_kandelo_init
//go:linkname x_cgo_kandelo_init x_cgo_kandelo_init
var x_cgo_kandelo_init byte

//go:cgo_import_static x_cgo_kandelo_get_channel_base
//go:linkname x_cgo_kandelo_get_channel_base x_cgo_kandelo_get_channel_base
var x_cgo_kandelo_get_channel_base byte

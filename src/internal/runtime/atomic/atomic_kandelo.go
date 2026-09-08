// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build kandelo

// Real WebAssembly atomic operations for the kandelo port.
//
// Unlike the js and wasip1 ports (see atomic_wasm.go), kandelo runs on SHARED
// linear memory, so it uses the genuine atomic memory instructions from the
// WebAssembly threads proposal (0xFE-prefix opcodes) instead of plain memory
// access. This makes the exported atomic API sound for a future multi-M
// scheduler. The low-level opcodes are emitted by the thin assembly helpers in
// atomic_kandelo.s; the wrappers below adapt them to the exact function set and
// signatures of atomic_wasm.go.
//
// Mapping notes:
//   - The wasm rmw.add/and/or/xchg ops return the PRIOR value. Xadd* therefore
//     recompute new = old + delta in Go to preserve the "return new" contract.
//   - rmw.cmpxchg returns the loaded value; Cas* succeed iff it equals old.
//   - 8-bit Xchg8 reuses the shared goXchg8 CAS loop (built on the real 32-bit
//     cmpxchg below), matching the other ports.
//   - StorepNoWB is a plain (non-atomic) unsynchronized store shared with the
//     other wasm ports via atomic_wasm.s.

// Export some functions via linkname to assembly in sync/atomic.
//
//go:linkname Load
//go:linkname Loadp
//go:linkname Load64
//go:linkname Loadint32
//go:linkname Loadint64
//go:linkname Loaduintptr
//go:linkname LoadAcquintptr
//go:linkname Xadd
//go:linkname Xaddint32
//go:linkname Xaddint64
//go:linkname Xadd64
//go:linkname Xadduintptr
//go:linkname Xchg
//go:linkname Xchg64
//go:linkname Xchgint32
//go:linkname Xchgint64
//go:linkname Xchguintptr
//go:linkname Cas
//go:linkname Cas64
//go:linkname Casint32
//go:linkname Casint64
//go:linkname Casuintptr
//go:linkname Store
//go:linkname Store64
//go:linkname Storeint32
//go:linkname Storeint64
//go:linkname Storeuintptr
//go:linkname StoreReluintptr

package atomic

import "unsafe"

// Low-level primitives implemented in atomic_kandelo.s. Each emits a single
// WebAssembly threads-proposal atomic instruction. rmw primitives return the
// prior value; the cmpxchg primitives return the loaded value.

//go:noescape
func atomicLoad32(ptr *uint32) uint32

//go:noescape
func atomicLoad64(ptr *uint64) uint64

//go:noescape
func atomicLoad8(ptr *uint8) uint32

//go:noescape
func atomicStore32(ptr *uint32, val uint32)

//go:noescape
func atomicStore64(ptr *uint64, val uint64)

//go:noescape
func atomicStore8(ptr *uint8, val uint32)

//go:noescape
func atomicAdd32(ptr *uint32, delta uint32) uint32

//go:noescape
func atomicAdd64(ptr *uint64, delta uint64) uint64

//go:noescape
func atomicAnd32(ptr *uint32, val uint32) uint32

//go:noescape
func atomicOr32(ptr *uint32, val uint32) uint32

//go:noescape
func atomicAnd8(ptr *uint8, val uint32)

//go:noescape
func atomicOr8(ptr *uint8, val uint32)

//go:noescape
func atomicXchg32(ptr *uint32, new uint32) uint32

//go:noescape
func atomicXchg64(ptr *uint64, new uint64) uint64

//go:noescape
func atomicCas32(ptr *uint32, old, new uint32) uint32

//go:noescape
func atomicCas64(ptr *uint64, old, new uint64) uint64

func atomicFence()

// Loads

//go:nosplit
//go:noinline
func Load(ptr *uint32) uint32 {
	return atomicLoad32(ptr)
}

//go:nosplit
//go:noinline
func Loadp(ptr unsafe.Pointer) unsafe.Pointer {
	// Pointers are 8 bytes wide on wasm (PtrSize == 8).
	return unsafe.Pointer(uintptr(atomicLoad64((*uint64)(ptr))))
}

//go:nosplit
//go:noinline
func LoadAcq(ptr *uint32) uint32 {
	return atomicLoad32(ptr)
}

//go:nosplit
//go:noinline
func LoadAcq64(ptr *uint64) uint64 {
	return atomicLoad64(ptr)
}

//go:nosplit
//go:noinline
func LoadAcquintptr(ptr *uintptr) uintptr {
	return uintptr(atomicLoad64((*uint64)(unsafe.Pointer(ptr))))
}

//go:nosplit
//go:noinline
func Load8(ptr *uint8) uint8 {
	return uint8(atomicLoad8(ptr))
}

//go:nosplit
//go:noinline
func Load64(ptr *uint64) uint64 {
	return atomicLoad64(ptr)
}

//go:nosplit
//go:noinline
func Loaduintptr(ptr *uintptr) uintptr {
	return uintptr(atomicLoad64((*uint64)(unsafe.Pointer(ptr))))
}

//go:nosplit
//go:noinline
func Loaduint(ptr *uint) uint {
	return uint(atomicLoad64((*uint64)(unsafe.Pointer(ptr))))
}

//go:nosplit
//go:noinline
func Loadint32(ptr *int32) int32 {
	return int32(atomicLoad32((*uint32)(unsafe.Pointer(ptr))))
}

//go:nosplit
//go:noinline
func Loadint64(ptr *int64) int64 {
	return int64(atomicLoad64((*uint64)(unsafe.Pointer(ptr))))
}

// Add / exchange

//go:nosplit
//go:noinline
func Xadd(ptr *uint32, delta int32) uint32 {
	return atomicAdd32(ptr, uint32(delta)) + uint32(delta)
}

//go:nosplit
//go:noinline
func Xadd64(ptr *uint64, delta int64) uint64 {
	return atomicAdd64(ptr, uint64(delta)) + uint64(delta)
}

//go:nosplit
//go:noinline
func Xadduintptr(ptr *uintptr, delta uintptr) uintptr {
	return uintptr(atomicAdd64((*uint64)(unsafe.Pointer(ptr)), uint64(delta))) + delta
}

//go:nosplit
//go:noinline
func Xaddint32(ptr *int32, delta int32) int32 {
	return int32(atomicAdd32((*uint32)(unsafe.Pointer(ptr)), uint32(delta))) + delta
}

//go:nosplit
//go:noinline
func Xaddint64(ptr *int64, delta int64) int64 {
	return int64(atomicAdd64((*uint64)(unsafe.Pointer(ptr)), uint64(delta))) + delta
}

//go:nosplit
//go:noinline
func Xchg(ptr *uint32, new uint32) uint32 {
	return atomicXchg32(ptr, new)
}

//go:nosplit
func Xchg8(addr *uint8, v uint8) uint8 {
	return goXchg8(addr, v)
}

//go:nosplit
//go:noinline
func Xchg64(ptr *uint64, new uint64) uint64 {
	return atomicXchg64(ptr, new)
}

//go:nosplit
//go:noinline
func Xchgint32(ptr *int32, new int32) int32 {
	return int32(atomicXchg32((*uint32)(unsafe.Pointer(ptr)), uint32(new)))
}

//go:nosplit
//go:noinline
func Xchgint64(ptr *int64, new int64) int64 {
	return int64(atomicXchg64((*uint64)(unsafe.Pointer(ptr)), uint64(new)))
}

//go:nosplit
//go:noinline
func Xchguintptr(ptr *uintptr, new uintptr) uintptr {
	return uintptr(atomicXchg64((*uint64)(unsafe.Pointer(ptr)), uint64(new)))
}

// Bitwise and / or

//go:nosplit
//go:noinline
func And8(ptr *uint8, val uint8) {
	atomicAnd8(ptr, uint32(val))
}

//go:nosplit
//go:noinline
func Or8(ptr *uint8, val uint8) {
	atomicOr8(ptr, uint32(val))
}

// NOTE: Do not add atomicxor8 (XOR is not idempotent).

//go:nosplit
//go:noinline
func And(ptr *uint32, val uint32) {
	atomicAnd32(ptr, val)
}

//go:nosplit
//go:noinline
func Or(ptr *uint32, val uint32) {
	atomicOr32(ptr, val)
}

// Compare-and-swap

//go:nosplit
//go:noinline
func Cas(ptr *uint32, old, new uint32) bool {
	return atomicCas32(ptr, old, new) == old
}

//go:nosplit
//go:noinline
func Cas64(ptr *uint64, old, new uint64) bool {
	return atomicCas64(ptr, old, new) == old
}

//go:nosplit
//go:noinline
func Casint32(ptr *int32, old, new int32) bool {
	return atomicCas32((*uint32)(unsafe.Pointer(ptr)), uint32(old), uint32(new)) == uint32(old)
}

//go:nosplit
//go:noinline
func Casint64(ptr *int64, old, new int64) bool {
	return atomicCas64((*uint64)(unsafe.Pointer(ptr)), uint64(old), uint64(new)) == uint64(old)
}

//go:nosplit
//go:noinline
func Casp1(ptr *unsafe.Pointer, old, new unsafe.Pointer) bool {
	return atomicCas64((*uint64)(unsafe.Pointer(ptr)), uint64(uintptr(old)), uint64(uintptr(new))) == uint64(uintptr(old))
}

//go:nosplit
//go:noinline
func Casuintptr(ptr *uintptr, old, new uintptr) bool {
	return atomicCas64((*uint64)(unsafe.Pointer(ptr)), uint64(old), uint64(new)) == uint64(old)
}

//go:nosplit
//go:noinline
func CasRel(ptr *uint32, old, new uint32) bool {
	return atomicCas32(ptr, old, new) == old
}

// Stores

//go:nosplit
//go:noinline
func Store(ptr *uint32, val uint32) {
	atomicStore32(ptr, val)
}

//go:nosplit
//go:noinline
func StoreRel(ptr *uint32, val uint32) {
	atomicStore32(ptr, val)
}

//go:nosplit
//go:noinline
func StoreRel64(ptr *uint64, val uint64) {
	atomicStore64(ptr, val)
}

//go:nosplit
//go:noinline
func StoreReluintptr(ptr *uintptr, val uintptr) {
	atomicStore64((*uint64)(unsafe.Pointer(ptr)), uint64(val))
}

//go:nosplit
//go:noinline
func Store8(ptr *uint8, val uint8) {
	atomicStore8(ptr, uint32(val))
}

//go:nosplit
//go:noinline
func Store64(ptr *uint64, val uint64) {
	atomicStore64(ptr, val)
}

//go:nosplit
//go:noinline
func Storeint32(ptr *int32, new int32) {
	atomicStore32((*uint32)(unsafe.Pointer(ptr)), uint32(new))
}

//go:nosplit
//go:noinline
func Storeint64(ptr *int64, new int64) {
	atomicStore64((*uint64)(unsafe.Pointer(ptr)), uint64(new))
}

//go:nosplit
//go:noinline
func Storeuintptr(ptr *uintptr, new uintptr) {
	atomicStore64((*uint64)(unsafe.Pointer(ptr)), uint64(new))
}

// StorepNoWB performs *ptr = val atomically and without a write
// barrier.
//
// It is a plain unsynchronized store shared with the other wasm ports via
// atomic_wasm.s.
//
// NO go:noescape annotation; see atomic_pointer.go.
func StorepNoWB(ptr unsafe.Pointer, val unsafe.Pointer)

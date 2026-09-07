// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package atomic_test

import (
	. "sync/atomic"
	"testing"
	"unsafe"
)

func TestUnaligned64Wasm32(t *testing.T) {
	x := [5]uint32{magic32, magic32, magic32, magic32, magic32}
	i := 1
	if uintptr(unsafe.Pointer(&x[i]))&7 == 0 {
		i++
	}
	p := (*uint64)(unsafe.Pointer(&x[i]))
	if uintptr(unsafe.Pointer(p))&7 != 4 {
		t.Fatal("pointer is not misaligned by 4 bytes")
	}
	// The two words at p are inside x, with a guard word on either side.
	t.Cleanup(func() {
		if x[i-1] != magic32 || x[i+2] != magic32 {
			t.Fatalf("wrong magic: %#x _ %#x != %#x _ %#x", x[i-1], x[i+2], magic32, magic32)
		}
	})
	check := func(name string, got, want, value uint64) {
		t.Helper()
		if got != want || *p != value {
			t.Fatalf("%s: got %#x, stored %#x; want %#x, %#x", name, got, *p, want, value)
		}
	}

	const initial uint64 = 0x12345678ffffffff
	const next uint64 = 0x89abcdefffffffff
	StoreUint64(p, initial)
	check("StoreUint64", *p, initial, initial)
	check("LoadUint64", LoadUint64(p), initial, initial)
	if CompareAndSwapUint64(p, initial^(1<<32), next) || *p != initial {
		t.Fatal("CompareAndSwapUint64 with mismatched old value succeeded or changed the value")
	}
	if !CompareAndSwapUint64(p, initial, next) || *p != next {
		t.Fatal("CompareAndSwapUint64 failed to replace the value")
	}
	check("AddUint64", AddUint64(p, 1), next+1, next+1)
	check("SwapUint64", SwapUint64(p, 0xffffffff00000000), next+1, 0xffffffff00000000)
	check("AndUint64", AndUint64(p, 0x8000000080000000), 0xffffffff00000000, 0x8000000000000000)
	check("OrUint64", OrUint64(p, 0x7fffffff00000001), 0x8000000000000000, 0xffffffff00000001)

	q := (*int64)(unsafe.Pointer(p))
	checkSigned := func(name string, got, want, value int64) {
		t.Helper()
		check(name, uint64(got), uint64(want), uint64(value))
	}
	const signedInitial int64 = -0x1234567800000001
	const signedNext int64 = -1 << 32
	StoreInt64(q, signedInitial)
	checkSigned("StoreInt64", *q, signedInitial, signedInitial)
	checkSigned("LoadInt64", LoadInt64(q), signedInitial, signedInitial)
	if CompareAndSwapInt64(q, signedInitial^(1<<32), signedNext) || *q != signedInitial {
		t.Fatal("CompareAndSwapInt64 with mismatched old value succeeded or changed the value")
	}
	if !CompareAndSwapInt64(q, signedInitial, signedNext) || *q != signedNext {
		t.Fatal("CompareAndSwapInt64 failed to replace the value")
	}
	checkSigned("AddInt64", AddInt64(q, -1), signedNext-1, signedNext-1)
	checkSigned("SwapInt64", SwapInt64(q, -1), signedNext-1, -1)
	checkSigned("AndInt64", AndInt64(q, -1<<32), -1, -1<<32)
	checkSigned("OrInt64", OrInt64(q, 1<<31), -1<<32, -1<<32|1<<31)
}

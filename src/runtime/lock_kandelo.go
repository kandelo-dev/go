// Copyright 2025 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build kandelo

package runtime

import (
	"internal/runtime/atomic"
	"unsafe"
)

const (
	mutex_unlocked = 0
	mutex_locked   = 1

	active_spin     = 4
	active_spin_cnt = 30
)

type mWaitList struct{}

func lockVerifyMSize() {}

func mutexContended(l *mutex) bool {
	return atomic.Load(kandeloKey32(&l.key)) != mutex_unlocked
}

//go:nosplit
func kandeloKey32(p *uintptr) *uint32 {
	return (*uint32)(unsafe.Pointer(p))
}

func lock(l *mutex) {
	lockWithRank(l, getLockRank(l))
}

func lock2(l *mutex) {
	gp := getg()
	if gp.m.locks < 0 {
		throw("lock count")
	}
	gp.m.locks++
	key := kandeloKey32(&l.key)
	for !atomic.Cas(key, mutex_unlocked, mutex_locked) {
		atomicWait32(key, mutex_locked, -1)
	}
}

func unlock(l *mutex) {
	unlockWithRank(l)
}

func unlock2(l *mutex) {
	key := kandeloKey32(&l.key)
	if atomic.Xchg(key, mutex_unlocked) == mutex_unlocked {
		throw("unlock of unlocked lock")
	}
	gp := getg()
	gp.m.locks--
	if gp.m.locks < 0 {
		throw("lock count")
	}
	atomicNotify(key, 1)
}

// One-time notifications.
func noteclear(n *note) {
	atomic.Store(kandeloKey32(&n.key), 0)
}

func notewakeup(n *note) {
	key := kandeloKey32(&n.key)
	if old := atomic.Xchg(key, 1); old != 0 {
		print("notewakeup - double wakeup (", old, ")\n")
		throw("notewakeup - double wakeup")
	}
	atomicNotify(key, 1)
}

func notesleep(n *note) {
	gp := getg()
	if gp != gp.m.g0 {
		throw("notesleep not on g0")
	}
	kandeloNoteSleep(n, -1)
}

func notetsleep(n *note, ns int64) bool {
	gp := getg()
	if gp != gp.m.g0 && gp.m.preemptoff != "" {
		throw("notetsleep not on g0")
	}
	return kandeloNoteSleep(n, ns)
}

//go:nosplit
//go:nowritebarrier
func kandeloNoteSleep(n *note, ns int64) bool {
	key := kandeloKey32(&n.key)
	if atomic.Load(key) != 0 {
		return true
	}
	deadline := int64(0)
	if ns >= 0 {
		deadline = nanotime() + ns
	}
	for atomic.Load(key) == 0 {
		remaining := int64(-1)
		if ns >= 0 {
			remaining = deadline - nanotime()
			if remaining <= 0 {
				return atomic.Load(key) != 0
			}
		}
		getg().m.blocked = true
		atomicWait32(key, 0, remaining)
		getg().m.blocked = false
	}
	return true
}

// same as runtime·notetsleep, but called on user g (not g0)
func notetsleepg(n *note, ns int64) bool {
	gp := getg()
	if gp == gp.m.g0 {
		throw("notetsleepg on g0")
	}

	entersyscallblock()
	ok := kandeloNoteSleep(n, ns)
	exitsyscall()
	return ok
}

func beforeIdle(int64, int64) (*g, bool) {
	return nil, false
}

func checkTimeouts() {}

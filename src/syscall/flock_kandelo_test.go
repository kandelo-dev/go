package syscall_test

import (
	"os"
	"syscall"
	"testing"
)

func TestKandeloFlock(t *testing.T) {
	first, err := os.CreateTemp("", "kandelo-flock-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(first.Name())
	defer first.Close()
	second, err := os.OpenFile(first.Name(), os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := syscall.Flock(int(first.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(second.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != syscall.EWOULDBLOCK {
		t.Fatalf("contending lock: got %v, want EWOULDBLOCK", err)
	}
	if err := syscall.Flock(int(first.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(second.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(second.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
}

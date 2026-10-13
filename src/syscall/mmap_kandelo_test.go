package syscall_test

import (
	"os"
	"syscall"
	"testing"
)

func TestKandeloMmap(t *testing.T) {
	file, err := os.CreateTemp("", "kandelo-mmap-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	pageSize := syscall.Getpagesize()
	if err := file.Truncate(int64(pageSize)); err != nil {
		t.Fatal(err)
	}

	shared, err := syscall.Mmap(int(file.Fd()), 0, pageSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		t.Fatal(err)
	}
	shared[0] = 'K'
	if err := syscall.Msync(shared, syscall.MS_SYNC); err != nil {
		t.Fatal(err)
	}
	stored := make([]byte, 1)
	if _, err := file.ReadAt(stored, 0); err != nil {
		t.Fatal(err)
	}
	if stored[0] != 'K' {
		t.Fatalf("shared msync: got %q, want K", stored[0])
	}
	if err := syscall.Munmap(shared[:1]); err != syscall.EINVAL {
		t.Fatalf("partial unmap: got %v, want EINVAL", err)
	}
	if err := syscall.Munmap(shared); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Munmap(shared); err != syscall.EINVAL {
		t.Fatalf("duplicate unmap: got %v, want EINVAL", err)
	}
	if _, err := file.ReadAt(stored, 0); err != nil {
		t.Fatal(err)
	}
	if stored[0] != 'K' {
		t.Fatalf("shared writeback: got %q, want K", stored[0])
	}

	private, err := syscall.Mmap(int(file.Fd()), 0, pageSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE)
	if err != nil {
		t.Fatal(err)
	}
	if private[0] != 'K' {
		t.Fatalf("private initial byte: got %q, want K", private[0])
	}
	private[0] = 'P'
	if err := syscall.Munmap(private); err != nil {
		t.Fatal(err)
	}
	if _, err := file.ReadAt(stored, 0); err != nil {
		t.Fatal(err)
	}
	if stored[0] != 'K' {
		t.Fatalf("private write leaked: got %q, want K", stored[0])
	}
	anonymous, err := syscall.Mmap(-1, 0, pageSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil {
		t.Fatal(err)
	}
	anonymous[0] = 'A'
	if anonymous[0] != 'A' {
		t.Fatal("anonymous mapping did not retain its write")
	}
	if err := syscall.Munmap(anonymous); err != nil {
		t.Fatal(err)
	}
	if _, err := syscall.Mmap(-1, 0, 0, syscall.PROT_READ, syscall.MAP_PRIVATE|syscall.MAP_ANON); err != syscall.EINVAL {
		t.Fatalf("zero-length map: got %v, want EINVAL", err)
	}
}

package syscall_test

import (
	"os"
	"syscall"
	"testing"
	"unsafe"
)

func TestKandeloTerminal(t *testing.T) {
	if size := unsafe.Sizeof(syscall.Termios{}); size != 60 {
		t.Fatalf("Termios size: got %d, want 60", size)
	}
	terminal, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	original, err := syscall.GetTermios(int(terminal.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	changed := *original
	changed.Lflag ^= syscall.ECHO
	if err := syscall.SetTermios(int(terminal.Fd()), &changed); err != nil {
		t.Fatal(err)
	}
	defer syscall.SetTermios(int(terminal.Fd()), original)
	current, err := syscall.GetTermios(int(terminal.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if current.Lflag&syscall.ECHO != changed.Lflag&syscall.ECHO {
		t.Fatalf("terminal echo flag: got %o, want %o", current.Lflag, changed.Lflag)
	}
	if _, err := syscall.GetWinsize(int(terminal.Fd())); err != nil {
		t.Fatal(err)
	}
	file, err := os.CreateTemp("", "kandelo-not-terminal-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := syscall.GetTermios(int(file.Fd())); err != syscall.ENOTTY {
		t.Fatalf("regular file terminal state: got %v, want ENOTTY", err)
	}
}

func TestKandeloRlimit(t *testing.T) {
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatal(err)
	}
	if limit.Cur == 0 || limit.Max < limit.Cur {
		t.Fatalf("file descriptor limit: %+v", limit)
	}
	if err := syscall.Getrlimit(syscall.RLIMIT_MEMLOCK, &limit); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Getrlimit(16, &limit); err != syscall.EINVAL {
		t.Fatalf("invalid resource: got %v, want EINVAL", err)
	}
}

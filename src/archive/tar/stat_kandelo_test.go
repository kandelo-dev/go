//go:build kandelo

package tar

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestKandeloFileInfoHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	atime := time.Unix(1_700_000_000, 123_456_789)
	mtime := time.Unix(1_700_000_001, 987_654_321)
	if err := os.Chtimes(path, atime, mtime); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	header, err := FileInfoHeader(info, "")
	if err != nil {
		t.Fatal(err)
	}
	stat := info.Sys().(*syscall.Stat_t)
	if header.AccessTime.UnixNano() != int64(stat.Atime) ||
		header.ChangeTime.UnixNano() != int64(stat.Ctime) ||
		header.Uid != int(stat.Uid) || header.Gid != int(stat.Gid) {
		t.Fatalf("header metadata does not match stat: %#v, %#v", header, stat)
	}
	device, err := os.Stat("/dev/kandelo/clipboard")
	if err != nil {
		t.Fatal(err)
	}
	deviceHeader, err := FileInfoHeader(device, "")
	if err != nil {
		t.Fatal(err)
	}
	if deviceHeader.Devmajor != 10 || deviceHeader.Devminor != 250 {
		t.Fatalf("clipboard device number = %d:%d, want 10:250", deviceHeader.Devmajor, deviceHeader.Devminor)
	}
}

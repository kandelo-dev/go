//go:build kandelo

package syscall

import (
	"runtime"
	"unsafe"
)

type Termios struct {
	Iflag  uint32
	Oflag  uint32
	Cflag  uint32
	Lflag  uint32
	Line   uint8
	Cc     [32]uint8
	_      [3]uint8
	Ispeed uint32
	Ospeed uint32
}

type Winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

const (
	IGNBRK     = 0o0000001
	BRKINT     = 0o0000002
	PARMRK     = 0o0000010
	ISTRIP     = 0o0000040
	INLCR      = 0o0000100
	IGNCR      = 0o0000200
	ICRNL      = 0o0000400
	IXON       = 0o0002000
	ECHO       = 0o0000010
	ECHONL     = 0o0000100
	ICANON     = 0o0000002
	ISIG       = 0o0000001
	IEXTEN     = 0o0100000
	CSIZE      = 0o0000060
	PARENB     = 0o0000400
	CS8        = 0o0000060
	VMIN       = 6
	VTIME      = 5
	TCGETS     = 0x5401
	TCSETS     = 0x5402
	TIOCGWINSZ = 0x5413
)

func GetTermios(fd int) (*Termios, error) {
	termios := new(Termios)
	_, errno := kandeloSyscall6(kSysTcgetattr, int64(fd), int64(uintptr(unsafe.Pointer(termios))), 0, 0, 0, 0)
	runtime.KeepAlive(termios)
	if errno != 0 {
		return nil, kandeloErrno(errno)
	}
	return termios, nil
}

func SetTermios(fd int, termios *Termios) error {
	if termios == nil {
		return EINVAL
	}
	_, errno := kandeloSyscall6(kSysTcsetattr, int64(fd), 0, int64(uintptr(unsafe.Pointer(termios))), 0, 0, 0)
	runtime.KeepAlive(termios)
	return errnoErr(kandeloErrno(errno))
}

func GetWinsize(fd int) (*Winsize, error) {
	winsize := new(Winsize)
	_, errno := kandeloSyscall6(kSysIoctl, int64(fd), TIOCGWINSZ, int64(uintptr(unsafe.Pointer(winsize))), int64(unsafe.Sizeof(*winsize)), 0, 0)
	runtime.KeepAlive(winsize)
	if errno != 0 {
		return nil, kandeloErrno(errno)
	}
	return winsize, nil
}

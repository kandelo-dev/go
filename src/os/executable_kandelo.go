//go:build kandelo

package os

func executable() (string, error) {
	return Readlink("/proc/self/exe")
}

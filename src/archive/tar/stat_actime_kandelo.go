//go:build kandelo

package tar

import (
	"syscall"
	"time"
)

func statAtime(stat *syscall.Stat_t) time.Time {
	return time.Unix(0, int64(stat.Atime))
}

func statCtime(stat *syscall.Stat_t) time.Time {
	return time.Unix(0, int64(stat.Ctime))
}

package service

import (
	"os"
	"syscall"
	"time"
)

// changeTime is the inode change time. Unlike mtime it cannot be set from
// userspace, so restoring an archive's mtime after rewriting it still moves it.
func changeTime(fi os.FileInfo) (time.Time, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(st.Ctimespec.Sec, st.Ctimespec.Nsec), true
}

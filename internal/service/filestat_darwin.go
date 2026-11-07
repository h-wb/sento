package service

import (
	"os"
	"syscall"
	"time"
)

// inodeStat reads the identity and change time a stat signature needs.
func inodeStat(fi os.FileInfo) (dev, ino uint64, ctime time.Time, ok bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, time.Time{}, false
	}
	return uint64(st.Dev), st.Ino, time.Unix(st.Ctimespec.Sec, st.Ctimespec.Nsec), true
}

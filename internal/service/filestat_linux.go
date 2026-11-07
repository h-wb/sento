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
	// Field widths differ between architectures.
	return uint64(st.Dev), uint64(st.Ino), time.Unix(int64(st.Ctim.Sec), int64(st.Ctim.Nsec)), true
}

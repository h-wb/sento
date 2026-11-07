//go:build !linux && !darwin

package service

import (
	"os"
	"time"
)

// changeTime reports no inode change time; callers fingerprint instead.
func changeTime(os.FileInfo) (time.Time, bool) { return time.Time{}, false }

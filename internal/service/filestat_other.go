//go:build !linux && !darwin

package service

import (
	"os"
	"time"
)

// inodeStat reports no inode metadata; archives are always fingerprinted.
func inodeStat(os.FileInfo) (dev, ino uint64, ctime time.Time, ok bool) {
	return 0, 0, time.Time{}, false
}

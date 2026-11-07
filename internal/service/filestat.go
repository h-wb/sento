package service

import (
	"fmt"
	"os"
	"time"
)

// racyStat is how long after a change an archive's stat signature becomes
// trustworthy. Until then a later write could share the timestamps a coarse
// filesystem clock gave the earlier one, as with git's racy index entries.
const racyStat = 2 * time.Second

// statArchive reads an archive's metadata through an open descriptor. On NFS
// the open revalidates attributes the client could otherwise serve from cache.
func statArchive(path string) (os.FileInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return f.Stat()
}

// statSignature identifies an archive's bytes without reading them: device,
// inode, size, mtime and ctime. Userspace cannot set ctime, so restoring the
// mtime after a rewrite still changes the signature. Empty when the platform
// reports no inode metadata.
func statSignature(fi os.FileInfo) string {
	dev, ino, ctime, ok := inodeStat(fi)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d:%d:%d:%d:%d", dev, ino, fi.Size(), fi.ModTime().UnixNano(), ctime.UnixNano())
}

// trustedSignature is the signature of metadata read after now, or empty
// while a later write could still produce the same one.
func trustedSignature(fi os.FileInfo, now time.Time) string {
	_, _, ctime, ok := inodeStat(fi)
	settled := now.Add(-racyStat)
	if !ok || !fi.ModTime().Before(settled) || !ctime.Before(settled) {
		return ""
	}
	return statSignature(fi)
}

package mediafile

import (
	"archive/zip"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func IsPageImage(name string) bool { return isImage(name) && !junk(name) }

// WriteImageArchive builds a staged archive from one book's loose pages.
// Original files are read only. The caller owns the temporary destination
// and installs it through the normal exclusive import path after validation.
func WriteImageArchive(ctx context.Context, dest string, pages []string) (err error) {
	if len(pages) == 0 {
		return fmt.Errorf("image folder has no pages")
	}
	pages = append([]string(nil), pages...)
	sort.Slice(pages, func(i, j int) bool { return naturalPageLess(filepath.Base(pages[i]), filepath.Base(pages[j])) })
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
		if err != nil {
			_ = os.Remove(dest)
		}
	}()
	zw := zip.NewWriter(f)
	for i, path := range pages {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = writeImagePage(zw, path, i); err != nil {
			return err
		}
	}
	if err = zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

func writeImagePage(zw *zip.Writer, path string, index int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("page is not a regular file: %s", filepath.Base(path))
	}
	head := make([]byte, 512)
	n, err := f.Read(head)
	if err != nil && err != io.EOF {
		return err
	}
	mime := http.DetectContentType(head[:n])
	ext := ""
	switch mime {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/gif":
		ext = ".gif"
	case "image/webp":
		ext = ".webp"
	case "image/bmp":
		ext = ".bmp"
	default:
		return fmt.Errorf("page %s is not a supported image (%s)", filepath.Base(path), mime)
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if ext == ".png" || ext == ".jpg" || ext == ".gif" {
		config, _, e := image.DecodeConfig(f)
		if e != nil || config.Width <= 0 || config.Height <= 0 {
			return fmt.Errorf("invalid page image: %s", filepath.Base(path))
		}
		if _, err = f.Seek(0, io.SeekStart); err != nil {
			return err
		}
	}
	out, err := zw.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("%05d%s", index+1, ext), Method: zip.Store})
	if err != nil {
		return err
	}
	_, err = io.Copy(out, f)
	return err
}

// Compare digit runs numerically without converting arbitrary-length runs
// to integers. The literal spelling is the final tie breaker (01 vs 1).
func naturalPageLess(a, b string) bool {
	x, y := strings.ToLower(a), strings.ToLower(b)
	for len(x) > 0 && len(y) > 0 {
		if x[0] >= '0' && x[0] <= '9' && y[0] >= '0' && y[0] <= '9' {
			i, j := 0, 0
			for i < len(x) && x[i] >= '0' && x[i] <= '9' {
				i++
			}
			for j < len(y) && y[j] >= '0' && y[j] <= '9' {
				j++
			}
			n, m := strings.TrimLeft(x[:i], "0"), strings.TrimLeft(y[:j], "0")
			if len(n) != len(m) {
				return len(n) < len(m)
			}
			if n != m {
				return n < m
			}
			x, y = x[i:], y[j:]
			continue
		}
		if x[0] != y[0] {
			return x[0] < y[0]
		}
		x, y = x[1:], y[1:]
	}
	if len(x) != len(y) {
		return len(x) < len(y)
	}
	return a < b
}

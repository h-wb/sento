package provider

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"golang.org/x/sync/errgroup"
)

func (p *SourceProvider) fetchChapter(ctx context.Context, externalID string, job *sourceJob) error {
	sourceID, chapterKey, err := parseSourceExternalID(externalID)
	if err != nil {
		return err
	}
	pages, err := p.host.Pages(ctx, sourceID, chapterKey)
	if err != nil {
		return fmt.Errorf("listing pages: %w", err)
	}
	if len(pages) == 0 {
		return fmt.Errorf("chapter has no pages")
	}
	p.mu.Lock()
	job.total = len(pages)
	p.mu.Unlock()

	if err := os.MkdirAll(p.stagingDir, 0o755); err != nil {
		return err
	}

	// Fetch pages concurrently into temp files, bounded by pageConc and
	// paced by a per-source rate limiter. The CBZ is then assembled in
	// page order, so the archive is byte-identical to a sequential fetch.
	fetched, err := p.fetchPages(ctx, sourceID, pages, job)
	if err != nil {
		return err
	}
	defer func() {
		for _, f := range fetched {
			if f.path != "" {
				_ = os.Remove(f.path)
			}
		}
	}()

	dest := p.stagedPath(externalID)
	tmp, err := os.CreateTemp(p.stagingDir, ".part-*.cbz")
	if err != nil {
		return err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	zw := zip.NewWriter(tmp)
	for i, f := range fetched {
		if err := writePageFile(zw, f, i); err != nil {
			return fmt.Errorf("page %d/%d: %w", i+1, len(pages), err)
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dest)
}

// fetchedPage is one downloaded page staged to a temp file, with the image
// extension already sniffed from its leading bytes.
type fetchedPage struct {
	path string
	ext  string
}

// fetchPages downloads every page concurrently (up to pageConc in flight)
// into temp files, pacing requests through the per-source limiter. It
// returns results in page order; on any error every temp file written so
// far is removed before returning.
func (p *SourceProvider) fetchPages(ctx context.Context, sourceID string, pages []string, job *sourceJob) ([]fetchedPage, error) {
	out := make([]fetchedPage, len(pages))
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(p.pageConc)

	var done int
	for i, ref := range pages {
		i, ref := i, ref
		g.Go(func() error {
			f, err := p.retryPage(ctx, ref, sourceID)
			if err != nil {
				return fmt.Errorf("page %d: %w", i+1, err)
			}
			out[i] = f
			p.mu.Lock()
			done++
			job.done = done
			p.mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		for _, f := range out {
			if f.path != "" {
				_ = os.Remove(f.path)
			}
		}
		return nil, err
	}
	return out, nil
}

// fetchPage streams one page image to a temp file, sniffing its extension
// from the leading bytes.
func (p *SourceProvider) fetchPage(ctx context.Context, ref, sourceID string) (fetchedPage, error) {
	body, err := p.host.Page(ctx, sourceID, ref)
	if err != nil {
		return fetchedPage{}, err
	}
	defer func() { _ = body.Close() }()

	tmp, err := os.CreateTemp(p.stagingDir, ".page-*")
	if err != nil {
		return fetchedPage{}, err
	}
	defer func() { _ = tmp.Close() }()

	// Copy first so a truncated HTTP body remains a read error, rather
	// than being mistaken for a legitimately small image by ReadFull.
	if _, err := io.Copy(tmp, body); err != nil {
		_ = os.Remove(tmp.Name())
		return fetchedPage{}, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		_ = os.Remove(tmp.Name())
		return fetchedPage{}, err
	}
	head := make([]byte, 512)
	n, err := tmp.Read(head)
	if err != nil && err != io.EOF {
		_ = os.Remove(tmp.Name())
		return fetchedPage{}, err
	}
	if n == 0 {
		_ = os.Remove(tmp.Name())
		return fetchedPage{}, io.ErrUnexpectedEOF
	}
	ext := imageExt(head[:n])
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fetchedPage{}, err
	}
	return fetchedPage{path: tmp.Name(), ext: ext}, nil
}

// writePageFile copies a staged page into the archive, naming it by index
// with the sniffed extension.
func writePageFile(zw *zip.Writer, f fetchedPage, index int) error {
	src, err := os.Open(f.path)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	name := fmt.Sprintf("%04d%s", index+1, f.ext)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
	if err != nil {
		return err
	}
	_, err = io.Copy(w, src)
	return err
}

func imageExt(head []byte) string {
	switch http.DetectContentType(head) {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

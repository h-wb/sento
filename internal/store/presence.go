package store

import (
	"context"
	"path/filepath"
	"strings"
)

// SetFilePresence records the outcome of one presence check.
func (s *Store) SetFilePresence(ctx context.Context, fileID int64, state, detail string) error {
	_, err := s.db.ExecContext(ctx, s.q(`UPDATE files SET presence=?,presence_error=?,checked_at=? WHERE id=?`), state, detail, utcNow(), fileID)
	return err
}

// InChapterFolder reports whether path lies in the series' separate
// chapter folder. False when chapters share the series folder.
func (sr *Series) InChapterFolder(path string) bool {
	return sr.ChapterPath != "" && strings.HasPrefix(path, sr.ChapterPath+string(filepath.Separator))
}

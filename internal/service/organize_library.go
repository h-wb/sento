package service

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/h-wb/sento/internal/apperr"
)

// SeriesOrganizePlan is one series' part of a library-wide organize
// preview.
type SeriesOrganizePlan struct {
	SeriesID int64        `json:"seriesId"`
	Title    string       `json:"title"`
	Changes  []FileChange `json:"changes"`
}

// SeriesOrganizeResult is what an organize run did to one series.
type SeriesOrganizeResult struct {
	SeriesID int64    `json:"seriesId"`
	Title    string   `json:"title"`
	Renamed  int      `json:"renamed"`
	Retagged int      `json:"retagged"`
	Errors   []string `json:"errors"`
}

// LibraryOrganizeRequest is which series to organize and which changes
// to apply. No series means the whole library.
type LibraryOrganizeRequest struct {
	SeriesIDs []int64 `json:"seriesIds"`
	Rename    bool    `json:"rename"`
	Retag     bool    `json:"retag"`
}

// LibraryOrganizeStatus is the progress of the current or last
// library-wide organize run.
type LibraryOrganizeStatus struct {
	Queued     bool   `json:"queued"`
	Running    bool   `json:"running"`
	StartedAt  string `json:"startedAt,omitempty"`  // RFC3339
	FinishedAt string `json:"finishedAt,omitempty"` // RFC3339
	Total      int    `json:"total"`
	Done       int    `json:"done"`
	// Current is the series being organized.
	Current  string `json:"current,omitempty"`
	Renamed  int    `json:"renamed"`
	Retagged int    `json:"retagged"`
	// Series lists the series where something was changed or failed.
	Series []SeriesOrganizeResult `json:"series"`
	Error  string                 `json:"error,omitempty"`
}

type libraryOrganizer struct {
	mu      sync.Mutex
	pending *LibraryOrganizeRequest
	status  LibraryOrganizeStatus
}

// librarySeries is every series, or the requested ones, in library order.
func (im *Importer) librarySeries(ctx context.Context, ids []int64) ([]SeriesOrganizePlan, error) {
	all, err := im.store.ListSeries(ctx)
	if err != nil {
		return nil, err
	}
	out := []SeriesOrganizePlan{}
	for _, s := range all {
		if len(ids) == 0 || slices.Contains(ids, s.ID) {
			out = append(out, SeriesOrganizePlan{SeriesID: s.ID, Title: s.Title})
		}
	}
	return out, nil
}

// PlanOrganizeLibrary previews Organize across the library (or the given
// series), listing only the series it would change.
func (im *Importer) PlanOrganizeLibrary(ctx context.Context, ids []int64) ([]SeriesOrganizePlan, error) {
	series, err := im.librarySeries(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := []SeriesOrganizePlan{}
	for _, s := range series {
		changes, err := im.PlanOrganize(ctx, s.SeriesID)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(changes, func(c FileChange) bool { return c.Rename || c.Retag }) {
			s.Changes = changes
			out = append(out, s)
		}
	}
	return out, nil
}

// QueueLibraryOrganize records the next library organize run; the
// library-organize task carries it out. Only one run is held at a time.
func (im *Importer) QueueLibraryOrganize(req LibraryOrganizeRequest) (LibraryOrganizeStatus, error) {
	if !req.Rename && !req.Retag {
		return LibraryOrganizeStatus{}, apperr.Invalidf("choose renaming, re-tagging or both")
	}
	o := &im.organizer
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.status.Running || o.status.Queued {
		return o.status, apperr.Conflictf("the library is already being organized")
	}
	o.pending = &req
	o.status = LibraryOrganizeStatus{Queued: true, Series: []SeriesOrganizeResult{}}
	return o.status, nil
}

// LibraryOrganizeStatus reports the current or last organize run.
func (im *Importer) LibraryOrganizeStatus() LibraryOrganizeStatus {
	o := &im.organizer
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.status
	st.Series = slices.Clone(st.Series)
	if st.Series == nil {
		st.Series = []SeriesOrganizeResult{}
	}
	return st
}

// RunLibraryOrganize carries out the queued organize run, or organizes
// the whole library when nothing was queued (a run from System → Tasks).
// Series are organized one at a time, so imports interleave, and a run
// cut short is resumed by running it again: files already in place drop
// out of the plan.
func (im *Importer) RunLibraryOrganize(ctx context.Context) error {
	o := &im.organizer
	o.mu.Lock()
	req := LibraryOrganizeRequest{Rename: true, Retag: true}
	if o.pending != nil {
		req = *o.pending
	}
	o.pending = nil
	o.status = LibraryOrganizeStatus{Running: true, StartedAt: time.Now().UTC().Format(time.RFC3339), Series: []SeriesOrganizeResult{}}
	o.mu.Unlock()

	err := im.organizeLibrary(ctx, req)

	o.mu.Lock()
	o.status.Running = false
	o.status.Current = ""
	o.status.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		o.status.Error = err.Error()
	}
	o.mu.Unlock()
	return err
}

func (im *Importer) organizeLibrary(ctx context.Context, req LibraryOrganizeRequest) error {
	o := &im.organizer
	series, err := im.librarySeries(ctx, req.SeriesIDs)
	if err != nil {
		return err
	}
	o.mu.Lock()
	o.status.Total = len(series)
	o.mu.Unlock()
	var failed error
	for _, s := range series {
		if err := ctx.Err(); err != nil {
			return err
		}
		o.mu.Lock()
		o.status.Current = s.Title
		o.mu.Unlock()

		r := SeriesOrganizeResult{SeriesID: s.SeriesID, Title: s.Title, Errors: []string{}}
		res, err := im.Organize(ctx, s.SeriesID, req.Rename, req.Retag)
		if res != nil {
			r.Renamed, r.Retagged = res.Renamed, res.Retagged
			r.Errors = append(r.Errors, res.Errors...)
		}
		stopped := err != nil && ctx.Err() != nil
		if err != nil && !stopped {
			r.Errors = append(r.Errors, err.Error())
		}
		if len(r.Errors) > 0 && failed == nil {
			failed = errors.New("some files could not be organized")
		}

		o.mu.Lock()
		if !stopped {
			o.status.Done++
		}
		o.status.Renamed += r.Renamed
		o.status.Retagged += r.Retagged
		if r.Renamed+r.Retagged > 0 || len(r.Errors) > 0 {
			o.status.Series = append(o.status.Series, r)
		}
		o.mu.Unlock()
		if stopped {
			return errors.Join(failed, err)
		}
	}
	return failed
}

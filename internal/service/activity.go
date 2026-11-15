package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/h-wb/sento/internal/store"
)

// ActivityData is the same work viewed across the library or for one series.
// It is derived from queue, coverage and review facts, never stored as a feed.
type ActivityData struct {
	Downloads    []ActivityDownload `json:"downloads"`
	Reviews      []store.ReviewItem `json:"reviews"`
	Events       []store.Decision   `json:"events"`
	SeriesTitles map[int64]string   `json:"seriesTitles"`
}

type ActivityDownload struct {
	ImportedBooks   int                `json:"importedBooks,omitempty"`
	UnresolvedBooks []store.BookRef    `json:"unresolvedBooks,omitempty"`
	Job             store.QueueItem    `json:"job"`
	Files           []store.ReviewItem `json:"files"`
	Attention       bool               `json:"attention"`
	History         bool               `json:"history"`
	Resolution      string             `json:"resolution,omitempty"`
	Issue           string             `json:"issue"`
	Explanation     string             `json:"explanation"`
	ListingID       int64              `json:"listingId,omitempty"`
	ListingName     string             `json:"listingName,omitempty"`
	SourceName      string             `json:"sourceName,omitempty"`
}

func (l *Library) Activity(ctx context.Context, seriesID int64) (*ActivityData, error) {
	data := &ActivityData{Downloads: []ActivityDownload{}, Reviews: []store.ReviewItem{}, Events: []store.Decision{}, SeriesTitles: map[int64]string{}}
	jobs, err := l.activityJobs(ctx, seriesID, data)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(jobs, func(i, j int) bool { return jobs[i].ID > jobs[j].ID })
	reviews, err := l.ReviewItems(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	sets := &activitySets{files: map[int64][]store.File{}, bindings: map[int64][]store.Binding{}, ledgers: map[int64]*ledgerView{}, pending: activityPending(jobs), byRelease: map[string]int{}}
	for _, job := range jobs {
		if err := l.activityRow(ctx, data, job, reviews, sets); err != nil {
			return nil, err
		}
	}
	attachFileReviews(data, reviews, sets)
	if err := l.activityImportedBooks(ctx, data, sets); err != nil {
		return nil, err
	}
	if err := l.activityEvents(ctx, data, seriesID); err != nil {
		return nil, err
	}
	return data, nil
}

// activityJobs loads the queue rows Activity shows.
func (l *Library) activityJobs(ctx context.Context, seriesID int64, data *ActivityData) ([]store.QueueItem, error) {
	if seriesID > 0 {
		series, e := l.store.GetSeries(ctx, seriesID)
		if e != nil {
			return nil, e
		}
		data.SeriesTitles[seriesID] = series.Title
		jobs, err := l.store.SeriesQueueItems(ctx, seriesID)
		for i := range jobs {
			jobs[i].SeriesTitle = series.Title
		}
		return jobs, err
	}
	return l.store.ListQueueItems(ctx, false)
}

// activitySets carries the per-series loads Activity's passes share.
type activitySets struct {
	files     map[int64][]store.File
	bindings  map[int64][]store.Binding
	ledgers   map[int64]*ledgerView
	pending   map[string][]int64
	byRelease map[string]int
}

// activityRow builds one download row for a queue job.
func (l *Library) activityRow(ctx context.Context, data *ActivityData, job store.QueueItem, reviews []store.ReviewItem, sets *activitySets) error {
	data.SeriesTitles[job.SeriesID] = job.SeriesTitle
	if _, ok := sets.files[job.SeriesID]; !ok {
		var err error
		if sets.files[job.SeriesID], err = l.store.ListFiles(ctx, job.SeriesID); err != nil {
			return err
		}
		if sets.bindings[job.SeriesID], err = l.store.ListBindings(ctx, job.SeriesID); err != nil {
			return err
		}
	}
	withLiveProgress(l.registry, &job)
	job.CanTryAnother = CanTryAnother(job, sets.files[job.SeriesID])
	d := ActivityDownload{Job: job, Files: []store.ReviewItem{}, ListingID: jobListing(job)}
	snap := jobSnapshot(job)
	d.SourceName = snap.Site
	for _, binding := range sets.bindings[job.SeriesID] {
		if binding.ID == d.ListingID {
			d.ListingName = binding.MangaTitle
			if binding.SourceName != "" {
				d.SourceName = binding.SourceName
			}
			break
		}
	}
	d.History = job.Status == store.QueueImported || job.Status == store.QueueCancelled
	d.Attention = job.Status == store.QueueFailed || job.Status == store.QueueNeedsReview || job.Status == store.QueueDispatchUnknown || (job.Status == store.QueueDownloading && job.Stalled)
	hasQuestions := false
	for _, f := range sets.files[job.SeriesID] {
		if f.Facts.ReleaseKey == "" || f.Facts.ReleaseKey != job.ReleaseKey {
			continue
		}
		for _, r := range reviews {
			if r.Kind == "file" && r.SeriesID == job.SeriesID && r.ItemID == f.ID {
				hasQuestions = true
				break
			}
		}
	}
	d.Issue, d.Explanation = downloadIssue(job)
	if job.Status == store.QueueFailed && job.Protocol == "source" && d.Issue == "gone" && !hasQuestions {
		d.History, d.Attention, d.Job.CanTryAnother = true, false, false
		d.Resolution = "Release unavailable. Missing books remain wanted and will be searched again."
	}
	if job.Status == store.QueueFailed && !hasQuestions && !d.History {
		if _, ok := sets.ledgers[job.SeriesID]; !ok {
			ledgers, err := loadLedger(ctx, l.store, job.SeriesID)
			if err != nil {
				return err
			}
			sets.ledgers[job.SeriesID] = ledgers
		}
		d.Resolution = failedResolution(job, sets.ledgers[job.SeriesID], sets.pending)
		if d.Resolution != "" {
			d.History = true
			d.Attention = false
			d.Job.CanTryAnother = false
		}
	}
	d.Job.Snapshot = nil // Decisions and listing metadata above are the UI's compact view.
	key := fmt.Sprintf("%d/%s", job.SeriesID, job.ReleaseKey)
	if job.ReleaseKey != "" && !d.History {
		if _, exists := sets.byRelease[key]; !exists {
			sets.byRelease[key] = len(data.Downloads)
		}
	}
	data.Downloads = append(data.Downloads, d)
	return nil
}

// attachFileReviews moves each file review onto the download row of the
// release it arrived in; the rest stay in the review list.
func attachFileReviews(data *ActivityData, reviews []store.ReviewItem, sets *activitySets) {
	for _, review := range reviews {
		data.SeriesTitles[review.SeriesID] = review.SeriesTitle
		if review.Kind == "download" {
			continue
		} // The download row owns all recovery controls.
		attached := false
		if review.Kind == "file" {
			for _, f := range sets.files[review.SeriesID] {
				if f.ID == review.ItemID && f.Facts.ReleaseKey != "" {
					if idx, ok := sets.byRelease[fmt.Sprintf("%d/%s", review.SeriesID, f.Facts.ReleaseKey)]; ok {
						d := &data.Downloads[idx]
						d.Files = append(d.Files, review)
						d.Attention = true
						d.History = false
						d.Resolution = ""
						d.Job.CanTryAnother = false
						d.Issue = "files"
						d.Explanation = "Confirm the edition and books in these files."
						attached = true
					}
					break
				}
			}
		}
		if !attached {
			data.Reviews = append(data.Reviews, review)
		}
	}
}

// activityImportedBooks counts, per download held for review, how many of
// its requested books imported and which did not.
func (l *Library) activityImportedBooks(ctx context.Context, data *ActivityData, sets *activitySets) error {
	for i := range data.Downloads {
		d := &data.Downloads[i]
		if d.Job.Status == store.QueueNeedsReview && len(d.Files) > 0 {
			if _, ok := sets.ledgers[d.Job.SeriesID]; !ok {
				ledgers, err := loadLedger(ctx, l.store, d.Job.SeriesID)
				if err != nil {
					return err
				}
				sets.ledgers[d.Job.SeriesID] = ledgers
			}
			requested := queueBooks(&d.Job)
			linked, _ := releaseCoverage(d.Job.ReleaseKey, requested, sets.ledgers[d.Job.SeriesID], sets.files[d.Job.SeriesID])
			for _, ref := range requested {
				if linked[store.BookKey(ref.VersionID, ref.BookID)] {
					d.ImportedBooks++
				} else {
					d.UnresolvedBooks = append(d.UnresolvedBooks, ref)
				}
			}
			if d.ImportedBooks > 0 {
				d.Explanation = fmt.Sprintf("%d of %d requested books are imported.", d.ImportedBooks, len(requested))
				if len(d.UnresolvedBooks) > 0 {
					d.Explanation += fmt.Sprintf(" Still unmatched: %d.", len(d.UnresolvedBooks))
				}
			}
		}
		for _, f := range d.Files {
			if f.Status == "unknown" || f.Status == "unavailable" {
				d.Explanation = "Check that these files are accessible, then confirm their contents."
				break
			}
		}
	}
	return nil
}

// activityEvents loads the decision history and fills in the series titles
// it names.
func (l *Library) activityEvents(ctx context.Context, data *ActivityData, seriesID int64) error {
	events, err := l.store.ListDecisions(ctx, seriesID, 200)
	if err != nil {
		return err
	}
	if events == nil {
		events = []store.Decision{}
	}
	data.Events = events
	for _, event := range data.Events {
		if event.SeriesID == 0 {
			continue
		}
		if _, ok := data.SeriesTitles[event.SeriesID]; !ok {
			series, e := l.store.GetSeries(ctx, event.SeriesID)
			if e == nil {
				data.SeriesTitles[event.SeriesID] = series.Title
			}
		}
	}
	return nil
}

// A failed attempt is history when every requested book is already owned,
// no longer collected, or covered by a different live job. No queue facts or
// reservations change merely because someone opens Activity.
func failedResolution(job store.QueueItem, view *ledgerView, pending map[string][]int64) string {
	books, err := queueBooksErr(&job)
	if err != nil || len(books) == 0 {
		return ""
	}
	replacing := false
	for _, ref := range books {
		found := view.book(ref.VersionID, ref.BookID)
		if found == nil || (found.State == StateOwned || found.State == StateUpgradeEligible || found.State == StateUnmonitored) {
			continue
		}
		covered := false
		for _, id := range pending[store.BookKey(ref.VersionID, ref.BookID)] {
			if id != job.ID {
				covered = true
				replacing = true
				break
			}
		}
		if !covered {
			return ""
		}
	}
	if replacing {
		return "Covered by another download"
	}
	return "These books are already owned or no longer collected"
}

func activityPending(jobs []store.QueueItem) map[string][]int64 {
	pending := map[string][]int64{}
	for _, job := range jobs {
		switch job.Status {
		case store.QueueFailed, store.QueueImported, store.QueueCancelled:
			continue
		}
		for _, ref := range queueBooks(&job) {
			key := store.BookKey(ref.VersionID, ref.BookID)
			pending[key] = append(pending[key], job.ID)
		}
	}
	return pending
}

var activityHTTPStatus = regexp.MustCompile(`(?i)\b(?:HTTP(?: error| status)?|status(?: code)?)[ :=]+(401|403|404|410|429|503)\b`)

func downloadIssue(job store.QueueItem) (string, string) {
	if job.Status == store.QueueDispatchUnknown {
		return "dispatch", "Confirm whether this download reached the client."
	}
	if job.Stalled && job.Status == store.QueueDownloading {
		return "stalled", "No download progress for at least 20 minutes."
	}
	e := strings.ToLower(job.Error)
	status := ""
	if match := activityHTTPStatus.FindStringSubmatch(e); len(match) > 1 {
		status = match[1]
	}
	switch {
	case strings.Contains(e, "not a local file header"):
		return "archive_read", "The source could not read part of the archive."
	case strings.Contains(e, "chapter has no pages"):
		return "empty", "The source returned no pages. No file was downloaded."
	case strings.Contains(e, "damaged archive") || strings.Contains(e, "unreadable archive"):
		return "damaged", "The source archive is damaged."
	case status == "404" || status == "410":
		return "gone", "The source no longer provides this download or one of its pages. Its books still need a replacement."
	case status == "403" || status == "401":
		return "access", "The source did not allow this download."
	case status == "429":
		return "rate_limit", "The source is limiting requests."
	case strings.Contains(e, "timeout") || strings.Contains(e, "connection refused") || status == "503":
		return "connection", "The source or download client is temporarily unavailable."
	case strings.Contains(e, "permission denied") || strings.Contains(e, "no such file"):
		return "storage", "The downloaded files could not be accessed."
	}
	if job.Status == store.QueueNeedsReview {
		return "import", "The download needs attention before it can be imported."
	}
	if job.Error != "" {
		return "download", "The download could not finish."
	}
	return "", ""
}

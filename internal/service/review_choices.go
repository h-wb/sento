package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/h-wb/sento/internal/apperr"
	"github.com/h-wb/sento/internal/store"
)

type ReviewFileRequest struct {
	FileID int64 `json:"fileId"`
	// Nil asks the resolver; an explicit list confirms these contents.
	Books    []store.BookRef `json:"books" tstype:"BookRef[] | null"`
	Language *string         `json:"language,omitempty"`
	Group    *string         `json:"group,omitempty"`
}

type ReviewPlanRequest struct {
	VersionID int64               `json:"versionId,omitempty"`
	Files     []ReviewFileRequest `json:"files"`
}

type ReviewCopy struct {
	FileID  int64     `json:"fileId"`
	Path    string    `json:"path"`
	Group   string    `json:"group"`
	Source  string    `json:"source"`
	Pinned  bool      `json:"pinned"`
	Present bool      `json:"present"`
	Score   FileScore `json:"score"`
}

type ReviewBookPlan struct {
	Book                 store.BookRef `json:"book"`
	Label                string        `json:"label"`
	Incoming             FileScore     `json:"incoming"`
	Owners               []ReviewCopy  `json:"owners"`
	UseIncoming          bool          `json:"useIncoming"`
	CurrentSelected      bool          `json:"currentSelected"`
	RequestedReplacement bool          `json:"requestedReplacement"`
	Reason               string        `json:"reason"`
}

type ReviewFilePlan struct {
	FileID      int64            `json:"fileId"`
	Path        string           `json:"path"`
	Group       string           `json:"group"`
	Source      string           `json:"source"`
	Books       []ReviewBookPlan `json:"books"`
	Reason      string           `json:"reason"`
	TrashReason string           `json:"trashReason"`
}

type ReviewPlan struct {
	Token string           `json:"token"`
	Files []ReviewFilePlan `json:"files"`
}

type ReviewFileChoice struct {
	FileID int64           `json:"fileId"`
	Use    []store.BookRef `json:"use"`
	// keep retains unused bytes; trash journals a recoverable move; later
	// leaves the file and its review untouched.
	Disposition string `json:"disposition"`
}

type ReviewApplyRequest struct {
	Request ReviewPlanRequest  `json:"request"`
	Token   string             `json:"token"`
	Choices []ReviewFileChoice `json:"choices"`
}

type ReviewFileOutcome struct {
	FileID int64  `json:"fileId"`
	Status string `json:"status"`
	Note   string `json:"note"`
}

func (l *Library) PreviewReviewChoices(ctx context.Context, sid int64, req ReviewPlanRequest) (*ReviewPlan, error) {
	p, _, _, err := reviewChoices(ctx, l.store, sid, req, time.Time{})
	return p, err
}

// reviewChoices fingerprints an archive unless its trusted stat signature
// shows it unchanged since a fingerprint matched its recorded hash. That is
// the signature recorded with the hash, or, when bound to an earlier preview
// read at since, the one that preview read: the caller then requires this
// plan's token to equal that preview's, and the token covers each signature.
// Signatures are trusted relative to the preview's time, so an unchanged
// archive whose timestamps settle in between keeps its token.
func reviewChoices(ctx context.Context, st *store.Store, sid int64, req ReviewPlanRequest, since time.Time) (*ReviewPlan, string, *proposer, error) {
	if len(req.Files) == 0 || len(req.Files) > 500 {
		return nil, "", nil, apperr.Invalidf("select between 1 and 500 files")
	}
	revision, err := st.ReviewRevision(ctx, sid)
	if err != nil {
		return nil, "", nil, err
	}
	p, err := newProposer(ctx, st, sid)
	if err != nil {
		return nil, "", nil, err
	}
	cov, err := st.Coverage(ctx, sid)
	if err != nil {
		return nil, "", nil, err
	}
	pins, err := st.PinnedFiles(ctx, sid)
	if err != nil {
		return nil, "", nil, err
	}
	p.stats = map[int64]string{}
	bound := !since.IsZero()
	if !bound {
		since = time.Now()
	}
	out := &ReviewPlan{Files: []ReviewFilePlan{}}
	selected := map[string]bool{}
	digests, seen := map[int64]string{}, map[int64]bool{}
	for _, input := range req.Files {
		f, ok := p.files[input.FileID]
		if !ok || seen[f.ID] {
			return nil, "", nil, apperr.Invalidf("file is missing or selected more than once")
		}
		seen[f.ID] = true
		f.Facts = reviewCorrectedFacts(f.Facts, input)
		row := ReviewFilePlan{FileID: f.ID, Path: f.Path, Group: backedFacts(f.Facts, f.Claims).Group, Source: reviewSource(f), Books: []ReviewBookPlan{}}
		if f.Presence != "present" {
			row.Reason, row.TrashReason = "Recheck file storage before applying choices", "Recheck file storage before moving this archive"
		} else {
			// Stat before hashing, so a write during or after the hash
			// changes the signature the commit compares.
			info, e := statArchive(f.Path)
			if e != nil {
				return nil, "", nil, fmt.Errorf("read incoming archive: %w", e)
			}
			sig := trustedSignature(info, since)
			p.stats[f.ID] = sig
			if f.Hash != "" && sig != "" && (sig == f.HashStat || bound) {
				digests[f.ID] = f.Hash
			} else {
				digests[f.ID], err = fileHash(f.Path)
				if err != nil {
					return nil, "", nil, fmt.Errorf("read incoming archive: %w", err)
				}
				if f.Hash != "" && f.Hash != digests[f.ID] {
					return nil, "", nil, apperr.Conflictf("archive contents changed; rescan the file before reviewing it")
				}
			}
			// Legacy files may have no fingerprint yet. A known fingerprint
			// must match the evidence, never be renewed by confirmation.
			f.Hash = digests[f.ID]
			p.files[f.ID] = f
		}
		refs := input.Books
		if refs == nil {
			if req.VersionID > 0 {
				version, e := st.GetVersion(ctx, req.VersionID)
				if e != nil {
					return nil, "", nil, e
				}
				col := editionByID(p.editions, version.EditionID)
				if col == nil {
					return nil, "", nil, apperr.NotFoundf("collected version not found")
				}
				var note string
				refs, _, note, _ = p.matchSlots(f.ID, *col, version.ID)
				if len(refs) == 0 && row.Reason == "" {
					row.Reason = note
				}
			} else if proposal := p.propose(f.ID); proposal != nil {
				refs = proposal.Books
			}
		}
		if refs == nil && row.Reason == "" {
			row.Reason = "Choose the edition and books, or skip this incoming file"
		}
		if len(refs) > 2000 {
			return nil, "", nil, apperr.Invalidf("too many books in one archive")
		}
		keys := map[string]bool{}
		for _, ref := range refs {
			version, e := st.GetVersion(ctx, ref.VersionID)
			if e != nil {
				return nil, "", nil, e
			}
			col := editionByID(p.editions, version.EditionID)
			if col == nil || col.ID != ref.EditionID {
				return nil, "", nil, apperr.Invalidf("book mapping belongs to another series or version")
			}
			var book *store.Book
			for i := range p.books[col.ID] {
				if p.books[col.ID][i].ID == ref.BookID {
					book = &p.books[col.ID][i]
					break
				}
			}
			key := store.BookKey(ref.VersionID, ref.BookID)
			if book == nil || keys[key] {
				return nil, "", nil, apperr.Invalidf("book is missing or selected more than once")
			}
			keys[key] = true
			ref.Ledger, ref.Number = col.Type, book.SequenceNum
			prof := p.profiles.For(*version)
			b := ReviewBookPlan{Book: ref, Label: col.Type + " " + book.SequenceStr, Owners: []ReviewCopy{}, Incoming: judgeMappedFile(*col, prof, p.profiles.Formats, f, store.AssetCoverage{Evidence: "manual"})}
			b.Reason = b.Incoming.Reason
			for _, a := range cov {
				if a.VersionID != ref.VersionID || a.BookID != ref.BookID {
					continue
				}
				if a.FileID == f.ID {
					b.CurrentSelected = f.Presence == "present" && !f.NeedsReview
					continue
				}
				owner := p.files[a.FileID]
				b.Owners = append(b.Owners, ReviewCopy{FileID: owner.ID, Path: owner.Path, Group: backedFacts(owner.Facts, owner.Claims).Group, Source: reviewSource(owner), Pinned: a.Pinned, Present: owner.Presence == "present", Score: judgeMappedFile(*col, prof, p.profiles.Formats, owner, a)})
				if a.Pinned {
					b.Reason = "Unpin the existing copy before replacing it"
				}
				if owner.Presence == "unknown" || owner.Presence == "unavailable" || !a.Verified || owner.NeedsReview {
					b.Reason = "Recheck or finish reviewing the existing copy before replacing it"
				}
			}
			if job, ok := p.jobs[f.Facts.ReleaseKey]; ok {
				var snap ReleaseCandidate
				_ = json.Unmarshal(job.Snapshot, &snap)
				b.RequestedReplacement = slices.ContainsFunc(snap.RequestedReplacements, func(v store.BookRef) bool { return v.VersionID == ref.VersionID && v.BookID == ref.BookID })
			}
			b.UseIncoming = b.Reason == "" && (b.CurrentSelected || !slices.ContainsFunc(b.Owners, func(o ReviewCopy) bool { return o.Present }) || b.RequestedReplacement)
			slices.SortFunc(b.Owners, func(a, b ReviewCopy) int {
				if a.FileID < b.FileID {
					return -1
				}
				if a.FileID > b.FileID {
					return 1
				}
				return 0
			})
			if !b.Incoming.FactsKnown {
				b.UseIncoming = false
			}
			// Editing confirmed contents is not consent to adopt a copy that
			// was deselected, including files reviewed before review_action.
			if !f.NeedsReview {
				b.UseIncoming = b.CurrentSelected
			}
			// Several incoming archives may contain the same book. Choose at
			// most one by default; every other archive remains available.
			b.UseIncoming = b.UseIncoming && row.Reason == "" && !selected[key]
			if b.UseIncoming {
				selected[key] = true
			}
			row.Books = append(row.Books, b)
		}
		for _, a := range cov {
			if a.FileID == f.ID {
				row.TrashReason = "This archive is currently selected by a book or version; keep it instead of moving it to trash"
			}
		}
		if slices.Contains(pins, f.ID) {
			row.TrashReason = "Unpin this archive before moving it to trash"
		}
		out.Files = append(out.Files, row)
	}
	fresh, err := st.ReviewRevision(ctx, sid)
	if err != nil {
		return nil, "", nil, err
	}
	if fresh != revision {
		return nil, "", nil, apperr.Conflictf("review changed; refresh the preview")
	}
	raw, err := json.Marshal([]any{revision, req, out, digests, p.stats, since.UnixNano()})
	if err != nil {
		return nil, "", nil, err
	}
	hash := sha256.Sum256(raw)
	out.Token = strconv.FormatInt(since.UnixNano(), 10) + "." + hex.EncodeToString(hash[:])
	return out, revision, p, nil
}

// reviewTokenTime is the time the preview that issued token read its archives.
func reviewTokenTime(token string) (time.Time, bool) {
	at, _, ok := strings.Cut(token, ".")
	nanos, err := strconv.ParseInt(at, 10, 64)
	if !ok || err != nil || nanos <= 0 {
		return time.Time{}, false
	}
	return time.Unix(0, nanos), true
}

func reviewCorrectedFacts(f store.AssetFacts, input ReviewFileRequest) store.AssetFacts {
	f.Sources = maps.Clone(f.Sources)
	if f.Sources == nil {
		f.Sources = map[string]string{}
	}
	if input.Language != nil {
		f.Language = strings.TrimSpace(*input.Language)
		f.Sources["language"] = "manual"
	}
	if input.Group != nil {
		f.Group = strings.TrimSpace(*input.Group)
		f.Sources["group"] = "manual"
	}
	return f
}

func reviewSource(f store.File) string {
	facts := backedFacts(f.Facts, f.Claims)
	if facts.Site != "" {
		return facts.Site
	}
	return facts.Protocol
}

func (im *Importer) ApplyReviewChoices(ctx context.Context, sid int64, req ReviewApplyRequest) ([]ReviewFileOutcome, error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	since, ok := reviewTokenTime(req.Token)
	if !ok {
		return nil, apperr.Conflictf("review changed; refresh the preview before applying choices")
	}
	plan, revision, p, err := reviewChoices(ctx, im.store, sid, req.Request, since)
	if err != nil {
		return nil, err
	}
	if req.Token != plan.Token {
		return nil, apperr.Conflictf("review changed; refresh the preview before applying choices")
	}
	if len(req.Choices) != len(plan.Files) {
		return nil, apperr.Invalidf("choose an action for every file")
	}
	var writes []store.FileReviewWrite
	out := []ReviewFileOutcome{}
	used := map[string]bool{}
	displaced := map[int64]bool{}
	for i, row := range plan.Files {
		choice, input := req.Choices[i], req.Request.Files[i]
		if choice.FileID != row.FileID {
			return nil, apperr.Invalidf("file choices do not match the preview")
		}
		if choice.Disposition == "later" {
			out = append(out, ReviewFileOutcome{FileID: row.FileID, Status: "later"})
			continue
		}
		if choice.Disposition != "keep" && choice.Disposition != "trash" {
			return nil, apperr.Invalidf("choose keep, trash or later")
		}
		f := p.files[row.FileID]
		if f.Presence != "present" {
			return nil, apperr.Conflictf("recheck file storage before applying choices")
		}
		// reviewChoices has just matched the live archive with the recorded
		// evidence under im.mu; the commit checks it is still that archive.
		w := store.FileReviewWrite{FileID: f.ID, Facts: reviewCorrectedFacts(f.Facts, input), Hash: f.Hash, HashStat: p.stats[f.ID], Reset: input.Books != nil, Disposition: choice.Disposition}
		for _, b := range row.Books {
			w.Contents = append(w.Contents, b.Book)
			if !slices.Contains(choice.Use, b.Book) {
				continue
			}
			retaining := !f.NeedsReview && b.CurrentSelected
			if b.Reason != "" && !retaining {
				return nil, apperr.Conflictf("cannot use %s: %s", b.Label, b.Reason)
			}
			if row.Reason != "" {
				return nil, apperr.Conflictf("cannot use this archive: %s", row.Reason)
			}
			key := store.BookKey(b.Book.VersionID, b.Book.BookID)
			if used[key] {
				return nil, apperr.Invalidf("choose one incoming archive per book and version")
			}
			used[key] = true
			w.Selected = append(w.Selected, b.Book)
			if retaining {
				// A fact correction retains this mapping, including a copy
				// now refused by policy. It cannot displace another owner.
				continue
			}
			for _, owner := range b.Owners {
				w.Replacements = append(w.Replacements, store.AssetCoverage{FileID: owner.FileID, VersionID: b.Book.VersionID, BookID: b.Book.BookID})
				displaced[owner.FileID] = true
			}
		}
		if len(w.Selected) != len(choice.Use) {
			return nil, apperr.Invalidf("book choices do not match the preview")
		}
		if choice.Disposition == "trash" && (len(w.Selected) > 0 || row.TrashReason != "") {
			return nil, apperr.Conflictf("keep this shared or selected archive instead of moving it to trash")
		}
		writes = append(writes, w)
	}
	if err = im.store.CommitFileReviews(ctx, sid, revision, writes, func() error {
		return validateReviewFiles(ctx, writes, p.files)
	}); err != nil {
		return nil, err
	}
	for _, w := range writes {
		result := ReviewFileOutcome{FileID: w.FileID, Status: "kept"}
		if w.Disposition == "trash" {
			if err = im.trashFile(ctx, sid, CleanupFile{FileID: w.FileID, Path: p.files[w.FileID].Path, Reason: "Unused incoming file rejected in review"}, 30, w.Hash); err != nil {
				result.Status, result.Note = "trash_pending", err.Error()
			} else {
				result.Status = "trashed"
			}
		} else if len(w.Selected) > 0 {
			result.Status = "imported"
			if err = im.organizeReviewedFile(ctx, sid, w.FileID); err != nil {
				result.Note = "Choices saved; organizing the archive still needs to finish: " + err.Error()
			}
		}
		out = append(out, result)
	}
	coverage, e := im.store.Coverage(ctx, sid)
	if e != nil {
		out = append(out, ReviewFileOutcome{Status: "settlement_pending", Note: "Choices saved; checking remaining archive uses still needs attention: " + e.Error()})
		return out, nil
	}
	for _, a := range coverage {
		delete(displaced, a.FileID)
	}
	for id := range displaced {
		im.exileToReview(ctx, sid, p.files[id])
	}
	if err = im.SettleReviewedJobs(ctx, sid); err != nil {
		out = append(out, ReviewFileOutcome{Status: "settlement_pending", Note: "Choices saved; finishing the download still needs attention: " + err.Error()})
	}
	return out, nil
}

// Under the commit's decision locks, after any wait for another writer,
// confirm each archive is still the one apply matched: by its stat signature,
// or by fingerprint when that was not trusted. Fingerprinting every archive
// here would hold the series' write locks while each page is decompressed.
// The store keeps filesystem knowledge in the service.
func validateReviewFiles(ctx context.Context, writes []store.FileReviewWrite, files map[int64]store.File) error {
	for _, w := range writes {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := files[w.FileID].Path
		if w.HashStat != "" {
			info, err := statArchive(path)
			if err != nil {
				return err
			}
			if statSignature(info) != w.HashStat {
				return apperr.Conflictf("archive contents changed; rescan the file before reviewing it")
			}
			continue
		}
		digest, err := fileHash(path)
		if err != nil {
			return err
		}
		if digest != w.Hash {
			return apperr.Conflictf("archive contents changed; rescan the file before reviewing it")
		}
	}
	return nil
}

// Legacy accept/mapping clients get the same keep-existing default. Automated
// import continues to use ApplyUpgrades and the configured acquisition policy.
func (im *Importer) ApplySafeFileMapping(ctx context.Context, sid, fid int64, refs []store.BookRef, replace bool) (*FileMappingResult, error) {
	if refs == nil {
		refs = []store.BookRef{}
	}
	req := ReviewPlanRequest{Files: []ReviewFileRequest{{FileID: fid, Books: refs}}}
	plan, _, _, err := reviewChoices(ctx, im.store, sid, req, time.Time{})
	if err != nil {
		return nil, err
	}
	choice := ReviewFileChoice{FileID: fid, Disposition: "keep", Use: []store.BookRef{}}
	result := &FileMappingResult{}
	for _, b := range plan.Files[0].Books {
		use := b.CurrentSelected || (b.Reason == "" && (len(b.Owners) == 0 || b.RequestedReplacement || replace))
		if use {
			choice.Use = append(choice.Use, b.Book)
		}
		for _, o := range b.Owners {
			if use && !b.CurrentSelected {
				result.Replaced = append(result.Replaced, Replacement{FileID: o.FileID, Path: o.Path, BookID: b.Book.BookID, VersionID: b.Book.VersionID, FromScore: o.Score.Score, ToScore: b.Incoming.Score})
			} else {
				result.Kept = append(result.Kept, KeptClaim{BookID: b.Book.BookID, VersionID: b.Book.VersionID, HolderFileID: o.FileID, HolderPath: o.Path, HolderScore: o.Score.Score, Score: b.Incoming.Score})
			}
		}
	}
	out, err := im.ApplyReviewChoices(ctx, sid, ReviewApplyRequest{Request: req, Token: plan.Token, Choices: []ReviewFileChoice{choice}})
	if err != nil {
		return nil, err
	}
	for _, f := range out {
		if f.Note != "" {
			result.Notes = append(result.Notes, f.Note)
		}
	}
	return result, nil
}

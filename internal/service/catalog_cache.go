package service

import (
	"context"

	"github.com/h-wb/sento/internal/store"
)

// catalogCache memoizes the catalog rows one pass over a series touches.
// An organize run preloads them in bulk, so planning a series of any size
// costs a fixed handful of queries instead of several per file; everywhere
// else rows load lazily on first use, exactly as before.
type catalogCache struct {
	st       *store.Store
	seriesID int64

	books        map[int64]*store.Book
	editions     map[int64]*store.Edition
	editionList  []store.Edition
	versions     map[int64]*store.CollectedVersion
	versionList  []store.CollectedVersion
	seriesRaw    string
	seriesLoaded bool
	worksRaw     map[int64]string
}

func newCatalogCache(st *store.Store, seriesID int64) *catalogCache {
	return &catalogCache{
		st:       st,
		seriesID: seriesID,
		books:    map[int64]*store.Book{},
		editions: map[int64]*store.Edition{},
		versions: map[int64]*store.CollectedVersion{},
		worksRaw: map[int64]string{},
	}
}

// preload loads everything a series-wide pass needs in a fixed number of
// queries, whatever the number of files.
func (c *catalogCache) preload(ctx context.Context) error {
	books, err := c.st.ListSeriesBooks(ctx, c.seriesID)
	if err != nil {
		return err
	}
	for i := range books {
		c.books[books[i].ID] = &books[i]
	}
	raws, err := c.st.BookRawPayloads(ctx, c.seriesID)
	if err != nil {
		return err
	}
	c.worksRaw = raws
	if c.editionList, err = c.st.ListEditions(ctx, c.seriesID); err != nil {
		return err
	}
	for i := range c.editionList {
		c.editions[c.editionList[i].ID] = &c.editionList[i]
	}
	if c.versionList, err = c.st.ListVersions(ctx, c.seriesID); err != nil {
		return err
	}
	for i := range c.versionList {
		c.versions[c.versionList[i].ID] = &c.versionList[i]
	}
	seriesRaw, _, err := c.st.CatalogPayloads(ctx, c.seriesID, 0)
	if err != nil {
		return err
	}
	c.seriesRaw, c.seriesLoaded = seriesRaw, true
	return nil
}

func (c *catalogCache) book(ctx context.Context, id int64) (*store.Book, error) {
	if b, ok := c.books[id]; ok {
		return b, nil
	}
	b, err := c.st.GetBook(ctx, id)
	if err != nil {
		return nil, err
	}
	c.books[id] = b
	return b, nil
}

func (c *catalogCache) edition(ctx context.Context, id int64) (*store.Edition, error) {
	if e, ok := c.editions[id]; ok {
		return e, nil
	}
	e, err := c.st.GetEdition(ctx, id)
	if err != nil {
		return nil, err
	}
	c.editions[id] = e
	return e, nil
}

// editionsAll is the decorated edition list of the series, loaded once.
func (c *catalogCache) editionsAll(ctx context.Context) ([]store.Edition, error) {
	if c.editionList != nil {
		return c.editionList, nil
	}
	eds, err := c.st.ListEditions(ctx, c.seriesID)
	if err != nil {
		return nil, err
	}
	c.editionList = eds
	for i := range eds {
		c.editions[eds[i].ID] = &eds[i]
	}
	return eds, nil
}

// versionsAll is the series' collected versions, loaded once.
func (c *catalogCache) versionsAll(ctx context.Context) ([]store.CollectedVersion, error) {
	if c.versionList != nil {
		return c.versionList, nil
	}
	vs, err := c.st.ListVersions(ctx, c.seriesID)
	if err != nil {
		return nil, err
	}
	c.versionList = vs
	for i := range vs {
		c.versions[vs[i].ID] = &vs[i]
	}
	return vs, nil
}

func (c *catalogCache) version(ctx context.Context, id int64) (*store.CollectedVersion, error) {
	if v, ok := c.versions[id]; ok {
		return v, nil
	}
	v, err := c.st.GetVersion(ctx, id)
	if err != nil {
		return nil, err
	}
	c.versions[id] = v
	return v, nil
}

// payloads returns the raw catalog documents behind a book: the series'
// (read once per cache) and, when the book is catalog-linked, the book's
// own. Either may be empty.
func (c *catalogCache) payloads(ctx context.Context, bookID int64) (series, work string, err error) {
	if !c.seriesLoaded {
		series, _, err = c.st.CatalogPayloads(ctx, c.seriesID, 0)
		if err != nil {
			return "", "", err
		}
		c.seriesRaw, c.seriesLoaded = series, true
	}
	if bookID == 0 {
		return c.seriesRaw, "", nil
	}
	if w, ok := c.worksRaw[bookID]; ok {
		return c.seriesRaw, w, nil
	}
	series, work, err = c.st.CatalogPayloads(ctx, c.seriesID, bookID)
	if err != nil {
		return "", "", err
	}
	c.seriesRaw, c.seriesLoaded = series, true
	c.worksRaw[bookID] = work
	return series, work, nil
}

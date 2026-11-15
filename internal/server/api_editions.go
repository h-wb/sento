package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/service"
	"github.com/h-wb/sento/internal/store"
)

// Edition handlers: collected versions, custom editions and collections,
// catalog linking, and coverage.

// mountEditionsAPI registers the edition routes.
func (s *Server) mountEditionsAPI(r chi.Router) {
	r.Get("/catalog/editions", s.handleCatalogEditions)
	r.Get("/series/{id}/edition-overview", s.handleEditionOverview)
	r.Get("/series/{id}/versions", s.handleListVersions)
	r.Post("/series/{id}/versions", s.handleSaveVersion)
	r.Put("/series/{id}/versions/{vid}", s.handleSaveVersion)
	r.Delete("/series/{id}/versions/{vid}", s.handleDeleteVersion)
	r.Post("/series/{id}/editions", s.handleCreateEdition)
	r.Put("/series/{id}/editions/{eid}", s.handleUpdateEdition)
	r.Delete("/series/{id}/editions/{eid}", s.handleDeleteEdition)
	r.Put("/series/{id}/editions/{eid}/metadata", s.handleEditEditionMetadata)
	r.Post("/series/{id}/editions/{eid}/catalog-link", s.handleCatalogLink)
	r.Post("/series/{id}/editions/{eid}/merge", s.handleMergeEditions)
	r.Post("/series/{id}/collections/{cid}/separate", s.handleSeparateCollection)
	r.Get("/series/{id}/coverage", s.handleCoverage)
	r.Put("/series/{id}/coverage/pin", s.handlePinCoverage)
	r.Post("/series/{id}/reader-names", s.handleReaderNames)
}

func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	data, err := s.store.ListVersions(r.Context(), id)
	respondList(w, data, err)
}
func (s *Server) handleSaveVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var t store.CollectedVersion
	if !decodeBody(w, r, &t) {
		return
	}
	if r.Method == http.MethodPut {
		vid, ok := pathID(w, r, "vid")
		if !ok {
			return
		}
		t.ID = vid
	}
	vid, err := s.library.SaveVersion(r.Context(), id, t)
	if err != nil {
		writeErr(w, err)
		return
	}
	data, err := s.store.GetVersion(r.Context(), vid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}
func (s *Server) handleDeleteVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	vid, ok := pathID(w, r, "vid")
	if !ok {
		return
	}
	if err := s.store.DeleteVersion(r.Context(), id, vid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCreateEdition(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body CreateEditionBody
	if !decodeBody(w, r, &body) {
		return
	}
	col, err := s.library.CreateEdition(r.Context(), id, service.CreateEditionParams{
		EditionName: body.EditionName, Language: body.Language,
		Publisher: body.Publisher, Medium: body.Medium, Type: body.Type,
		ProviderEditionID: body.ProviderEditionID,
		BasedOn:           body.BasedOn, Volumes: body.Volumes,
		CutoffNum: body.CutoffNum, Monitored: body.Monitored,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, col)
}

func (s *Server) handleMergeEditions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cid, ok := pathID(w, r, "eid")
	if !ok {
		return
	}
	var body MergeEditionBody
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Into == 0 || body.Into == cid {
		writeError(w, http.StatusBadRequest, "choose a different edition to merge into")
		return
	}
	if r.URL.Query().Get("preview") == "1" {
		data, err := s.store.PreviewMerge(r.Context(), id, cid, body.Into)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, data)
		return
	}
	unmatched, err := s.store.MergeEditions(r.Context(), id, cid, body.Into)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, MergeEditionResponse{Unmatched: unmatched})
}

func (s *Server) handleDeleteEdition(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cid, ok := pathID(w, r, "eid")
	if !ok {
		return
	}
	if err := s.store.DeleteCustomEdition(r.Context(), id, cid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleReaderNames renders what readers show for each of a series'
// collected versions, previewing an edition's unsaved templates when the
// body names them.
func (s *Server) handleReaderNames(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var body service.ReaderNamesRequest
	if !decodeBody(w, r, &body) {
		return
	}
	names, err := s.library.ReaderNames(r.Context(), id, body)
	respondList(w, names, err)
}

func (s *Server) handleUpdateEdition(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cid, ok := pathID(w, r, "eid")
	if !ok {
		return
	}
	var body EditionPatch
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.library.UpdateEdition(r.Context(), id, cid, service.EditionUpdate{
		Monitored: body.Monitored,
		CutoffNum: body.CutoffNum, Language: body.Language,
		ReaderFields: body.ReaderFields,
	}); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEditEditionMetadata(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cid, ok := pathID(w, r, "eid")
	if !ok {
		return
	}
	var p store.EditionMetadataPatch
	if !decodeBody(w, r, &p) {
		return
	}
	if err := s.store.EditEdition(r.Context(), id, cid, p); err != nil {
		writeErr(w, err)
		return
	}
	if len(p.Reset) > 0 {
		if err := s.library.Refresh(r.Context(), id); err != nil {
			writeErr(w, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCatalogLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cid, ok := pathID(w, r, "eid")
	if !ok {
		return
	}
	var b CatalogLinkBody
	if !decodeBody(w, r, &b) {
		return
	}
	if r.URL.Query().Get("preview") == "1" {
		data, err := s.store.PreviewCatalogLink(r.Context(), id, cid, b.CatalogID)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, data)
		return
	}
	if r.URL.Query().Get("decline") == "1" {
		if err := s.store.DeclineCatalogLink(r.Context(), id, cid, b.CatalogID); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := s.store.LinkCatalog(r.Context(), id, cid, b.CatalogID); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleCatalogEditions lists the catalog's edition names.
func (s *Server) handleCatalogEditions(w http.ResponseWriter, r *http.Request) {
	editions, err := s.library.CatalogEditions(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, editions)
}

// handleSeparateCollection gives one collection an edition of its own.
func (s *Server) handleSeparateCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cid, ok := pathID(w, r, "cid")
	if !ok {
		return
	}
	edition, err := s.store.SeparateCollection(r.Context(), id, cid)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err = s.store.EnsureModels(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, SeparateCollectionResponse{EditionID: edition})
}

func (s *Server) handleCoverage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	data, err := s.store.Coverage(r.Context(), id)
	respondList(w, data, err)
}
func (s *Server) handlePinCoverage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var a store.AssetCoverage
	if !decodeBody(w, r, &a) {
		return
	}
	if err := s.store.PinCoverage(r.Context(), id, a); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleEditionOverview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	data, err := s.library.EditionOverview(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

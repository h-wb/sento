package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/h-wb/sento/internal/store"
)

// Book handlers: the books of a series and its versions, their monitoring,
// catalog works, splitting and observations.

// mountBooksAPI registers the book routes.
func (s *Server) mountBooksAPI(r chi.Router) {
	r.Get("/series/{id}/books", s.handleBooksByVersion)
	r.Get("/series/{id}/versions/{vid}/books", s.handleVersionBooks)
	r.Post("/series/{id}/versions/{vid}/monitor", s.handleBulkMonitor)
	r.Post("/series/{id}/books", s.handleSaveBook)
	r.Put("/series/{id}/books/{bid}", s.handleSaveBook)
	r.Delete("/series/{id}/books/{bid}", s.handleDeleteBook)
	r.Get("/series/{id}/books/{bid}/works", s.handleBookWorks)
	r.Post("/series/{id}/books/{bid}/catalog-reset", s.handleResetBook)
	r.Post("/series/{id}/books/{bid}/split", s.handleSplitBox)
	r.Put("/series/{id}/book-monitor", s.handleBookMonitor)
	r.Get("/series/{id}/observations", s.handleObservations)
	r.Post("/series/{id}/observations", s.handleObservations)
	r.Put("/series/{id}/observations/{oid}", s.handleResolveObservation)
}

func (s *Server) handleBooksByVersion(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	data, err := s.library.BooksByVersion(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}
func (s *Server) handleVersionBooks(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	vid, ok := pathID(w, r, "vid")
	if !ok {
		return
	}
	data, err := s.library.VersionBooks(r.Context(), id, vid)
	respondList(w, data, err)
}
func (s *Server) handleBulkMonitor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	vid, ok := pathID(w, r, "vid")
	if !ok {
		return
	}
	var body BulkMonitorBody
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.store.BulkMonitor(r.Context(), id, vid, body.BookIDs, body.Monitored); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) handleSaveBook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var unit store.Book
	if !decodeBody(w, r, &unit) {
		return
	}
	if r.Method == http.MethodPut {
		bid, ok := pathID(w, r, "bid")
		if !ok {
			return
		}
		unit.ID = bid
	}
	bid, err := s.store.SaveBook(r.Context(), id, unit)
	if err != nil {
		writeErr(w, err)
		return
	}
	data, err := s.store.GetBook(r.Context(), bid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}
func (s *Server) handleDeleteBook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	bid, ok := pathID(w, r, "bid")
	if !ok {
		return
	}
	if err := s.store.DeleteBook(r.Context(), id, bid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleBookWorks lists the catalog works behind one book.
func (s *Server) handleBookWorks(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	bid, ok := pathID(w, r, "bid")
	if !ok {
		return
	}
	book, err := s.store.GetBook(r.Context(), bid)
	if err == nil {
		var edition *store.Edition
		if edition, err = s.store.GetEdition(r.Context(), book.EditionID); err == nil && edition.SeriesID != id {
			err = store.ErrNotFound
		}
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	works, err := s.store.ListWorks(r.Context(), bid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, works)
}

func (s *Server) handleResetBook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	bid, ok := pathID(w, r, "bid")
	if !ok {
		return
	}
	if err := s.store.ResetBook(r.Context(), id, bid); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.library.Refresh(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSplitBox turns one catalog item into the box of volumes it is.
func (s *Server) handleSplitBox(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	bid, ok := pathID(w, r, "bid")
	if !ok {
		return
	}
	var body SplitBoxBody
	if !decodeBody(w, r, &body) {
		return
	}
	volumes, err := s.library.SplitBox(r.Context(), id, bid, body.Count)
	respondList(w, volumes, err)
}

func (s *Server) handleBookMonitor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var b BookMonitorBody
	if !decodeBody(w, r, &b) {
		return
	}
	if err := s.store.SetBookMonitored(r.Context(), id, b.VersionID, b.BookID, b.Monitored); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleObservations(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		data, err := s.store.ListObservations(r.Context(), id)
		respondList(w, data, err)
		return
	}
	var o store.BookObservation
	if !decodeBody(w, r, &o) {
		return
	}
	if err := s.store.ObserveBook(r.Context(), id, o); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResolveObservation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	oid, ok := pathID(w, r, "oid")
	if !ok {
		return
	}
	var b ObservationReviewBody
	if !decodeBody(w, r, &b) {
		return
	}
	if err := s.store.ResolveObservation(r.Context(), id, oid, b.Accept); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

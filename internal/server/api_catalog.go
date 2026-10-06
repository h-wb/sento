package server

import (
	"github.com/h-wb/sento/internal/store"
	"net/http"
	"strconv"
)

func (s *Server) handleListTargets(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	data, err := s.store.ListTargets(r.Context(), id)
	respondList(w, data, err)
}
func (s *Server) handleSaveTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var t store.DownloadTarget
	if !decodeBody(w, r, &t) {
		return
	}
	if r.Method == http.MethodPut {
		tid, ok := pathID(w, r, "tid")
		if !ok {
			return
		}
		t.ID = tid
	}
	tid, err := s.store.SaveTarget(r.Context(), id, t)
	if err != nil {
		writeErr(w, err)
		return
	}
	data, err := s.store.GetTarget(r.Context(), tid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}
func (s *Server) handleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	tid, ok := pathID(w, r, "tid")
	if !ok {
		return
	}
	if err := s.store.DeleteTarget(r.Context(), id, tid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) handleTargetSlots(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	data, err := s.library.TargetSlots(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}
func (s *Server) handleSaveUnit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var unit store.Work
	if !decodeBody(w, r, &unit) {
		return
	}
	if r.Method == http.MethodPut {
		uid, ok := pathID(w, r, "uid")
		if !ok {
			return
		}
		unit.ID = uid
	}
	uid, err := s.store.SaveWork(r.Context(), id, unit)
	if err != nil {
		writeErr(w, err)
		return
	}
	data, err := s.store.GetWork(r.Context(), uid)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}
func (s *Server) handleDeleteUnit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	uid, ok := pathID(w, r, "uid")
	if !ok {
		return
	}
	if err := s.store.DeleteWork(r.Context(), id, uid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) handleEditCollectionMetadata(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cid, ok := pathID(w, r, "cid")
	if !ok {
		return
	}
	var p store.CollectionMetadataPatch
	if !decodeBody(w, r, &p) {
		return
	}
	if err := s.store.EditCollection(r.Context(), id, cid, p); err != nil {
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

type CatalogLinkBody struct {
	CatalogID int64 `json:"catalogId"`
}

func (s *Server) handleCatalogLink(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	cid, ok := pathID(w, r, "cid")
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
	if err := s.store.LinkCatalog(r.Context(), id, cid, b.CatalogID); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
func (s *Server) handleRemapFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	fid, ok := pathID(w, r, "fid")
	if !ok {
		return
	}
	var body FileMappingBody
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.store.MapFile(r.Context(), id, fid, body.Slots); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type FileMappingBody struct {
	Slots []store.ExpectedSlot `json:"slots"`
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
	var o store.UnitObservation
	if !decodeBody(w, r, &o) {
		return
	}
	if err := s.store.ObserveUnit(r.Context(), id, o); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type ObservationReviewBody struct {
	Accept bool `json:"accept"`
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
func (s *Server) handleRelations(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		data, err := s.store.Relations(r.Context(), id)
		respondList(w, data, err)
		return
	}
	var b store.WorkRelation
	if !decodeBody(w, r, &b) {
		return
	}
	var err error
	if r.Method == http.MethodDelete {
		err = s.store.DeleteRelation(r.Context(), id, b)
	} else {
		err = s.store.SetRelation(r.Context(), id, b)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type UnitMonitorBody struct {
	TargetID  int64 `json:"targetId"`
	WorkID    int64 `json:"workId"`
	Monitored bool  `json:"monitored"`
}

func (s *Server) handleUnitMonitor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	var b UnitMonitorBody
	if !decodeBody(w, r, &b) {
		return
	}
	if err := s.store.SetUnitMonitored(r.Context(), id, b.TargetID, b.WorkID, b.Monitored); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResetUnit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	uid, ok := pathID(w, r, "uid")
	if !ok {
		return
	}
	if err := s.store.ResetWork(r.Context(), id, uid); err != nil {
		writeErr(w, err)
		return
	}
	if err := s.library.Refresh(r.Context(), id); err != nil {
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
func (s *Server) handleUnitPage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	tid, ok := pathID(w, r, "tid")
	if !ok {
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	data, err := s.library.TargetPage(r.Context(), id, tid, page, r.URL.Query().Get("filter"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, data)
}
func (s *Server) handleBulkMonitor(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	tid, ok := pathID(w, r, "tid")
	if !ok {
		return
	}
	var body struct {
		WorkIDs   []int64 `json:"workIds"`
		Monitored bool    `json:"monitored"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if err := s.store.BulkMonitor(r.Context(), id, tid, body.WorkIDs, body.Monitored); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	sid, _ := strconv.ParseInt(r.URL.Query().Get("seriesId"), 10, 64)
	data, err := s.store.ReviewItems(r.Context(), sid)
	respondList(w, data, err)
}

func (s *Server) handleFileFacts(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	fid, ok := pathID(w, r, "fid")
	if !ok {
		return
	}
	var p store.AssetFactPatch
	if !decodeBody(w, r, &p) {
		return
	}
	if err := s.store.CorrectFileFacts(r.Context(), sid, fid, p); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) handleCheckFiles(w http.ResponseWriter, r *http.Request) {
	sid, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.CheckFilePresence(r.Context(), sid); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

package api

import (
	"net/http"
	"strconv"
)

func (s *Server) handleMenu(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.menu.Menu(r.Context()))
}

func (s *Server) handleMenuItem(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Позиция не найдена")
		return
	}
	item, err := s.menu.Item(r.Context(), id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) handleItemRecommendations(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Позиция не найдена")
		return
	}
	sess, _ := SessionFrom(r.Context())
	view, err := s.recs.ForDish(r.Context(), sess.ID, id)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

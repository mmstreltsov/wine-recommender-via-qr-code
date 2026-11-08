package api

import (
	"net/http"
)

type clientEventRequest struct {
	Name    string         `json:"name"`
	Payload map[string]any `json:"payload"`
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	sess, ok := SessionFrom(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка сервера")
		return
	}
	if !s.eventsLimiter.Allow(sess.ID) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Слишком много запросов")
		return
	}
	var req clientEventRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректное тело запроса")
		return
	}
	if err := s.events.RecordClientEvent(r.Context(), sess.ID, sess.TableID, req.Name, req.Payload); err != nil {
		writeAppError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

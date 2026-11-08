package api

import (
	"encoding/json"
	"net/http"
	"strconv"
)

const maxBodyBytes = 64 * 1024

type addCartItemRequest struct {
	ItemID *int64 `json:"item_id"`
	Source string `json:"source"`
}

type setQtyRequest struct {
	Qty *int `json:"qty"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	return json.NewDecoder(r.Body).Decode(v)
}

func (s *Server) handleCartGet(w http.ResponseWriter, r *http.Request) {
	sess, _ := SessionFrom(r.Context())
	view, err := s.cart.View(r.Context(), sess.ID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleCartAdd(w http.ResponseWriter, r *http.Request) {
	var req addCartItemRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректное тело запроса")
		return
	}
	if req.ItemID == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректное тело запроса")
		return
	}
	sess, _ := SessionFrom(r.Context())
	view, err := s.cart.Add(r.Context(), sess.ID, sess.TableID, *req.ItemID, req.Source)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, view)
}

func (s *Server) handleCartSetQty(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseInt(r.PathValue("item_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Позиция не найдена в корзине")
		return
	}
	var req setQtyRequest
	if err := decodeJSON(w, r, &req); err != nil || req.Qty == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "Некорректное тело запроса")
		return
	}
	sess, _ := SessionFrom(r.Context())
	view, err := s.cart.SetQty(r.Context(), sess.ID, sess.TableID, itemID, *req.Qty)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleCartRemove(w http.ResponseWriter, r *http.Request) {
	itemID, err := strconv.ParseInt(r.PathValue("item_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Позиция не найдена в корзине")
		return
	}
	sess, _ := SessionFrom(r.Context())
	view, err := s.cart.Remove(r.Context(), sess.ID, sess.TableID, itemID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

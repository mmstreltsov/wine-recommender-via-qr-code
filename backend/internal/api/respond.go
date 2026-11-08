package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"svoyshef/internal/app"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorEnvelope{Error: errorBody{Code: code, Message: message}})
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	ItemID  *int64 `json:"item_id,omitempty"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

func writeAppError(w http.ResponseWriter, err error) {
	var ue *app.UserError
	if errors.As(err, &ue) {
		status := http.StatusInternalServerError
		switch ue.Code {
		case app.CodeBadRequest:
			status = http.StatusBadRequest
		case app.CodeNotFound:
			status = http.StatusNotFound
		case app.CodeItemUnavailable:
			status = http.StatusConflict
		}
		writeJSON(w, status, errorEnvelope{Error: errorBody{Code: ue.Code, Message: ue.Message, ItemID: ue.ItemID}})
		return
	}
	log.Printf("internal error: %v", err)
	writeError(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка сервера")
}

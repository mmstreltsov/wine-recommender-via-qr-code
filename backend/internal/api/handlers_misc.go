package api

import (
	"net/http"
)

type RestaurantConfig struct {
	Name    string `json:"name"`
	LogoURL string `json:"logo_url"`
}

type ConfigResponse struct {
	Restaurant    RestaurantConfig `json:"restaurant"`
	RecBlockTitle string           `json:"rec_block_title"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Status     string `json:"status"`
		Restaurant string `json:"restaurant"`
	}{Status: "ok", Restaurant: s.restaurant})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, ConfigResponse{Restaurant: s.config, RecBlockTitle: s.recBlockTitle})
}

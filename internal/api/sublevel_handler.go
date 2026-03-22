package api

import (
	"encoding/json"
	"net/http"

	"github.com/openilink/openilink-hub/internal/auth"
	"github.com/openilink/openilink-hub/internal/database"
)

func (s *Server) handleListSublevels(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	subs, err := s.DB.ListSublevelsByUser(userID)
	if err != nil {
		jsonError(w, "list failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(subs)
}

func (s *Server) handleCreateSublevel(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	var req struct {
		BotID      string               `json:"bot_id"`
		Name       string               `json:"name"`
		FilterRule *database.FilterRule  `json:"filter_rule,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BotID == "" || req.Name == "" {
		jsonError(w, "bot_id and name required", http.StatusBadRequest)
		return
	}

	bot, err := s.DB.GetBot(req.BotID)
	if err != nil || bot.UserID != userID {
		jsonError(w, "bot not found", http.StatusNotFound)
		return
	}

	sub, err := s.DB.CreateSublevel(userID, req.BotID, req.Name, req.FilterRule)
	if err != nil {
		jsonError(w, "create failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sub)
}

func (s *Server) handleUpdateSublevel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())

	sub, err := s.DB.GetSublevel(id)
	if err != nil || sub.UserID != userID {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	var req struct {
		Name       string              `json:"name"`
		FilterRule *database.FilterRule `json:"filter_rule"`
		Enabled    *bool               `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}

	name := sub.Name
	if req.Name != "" {
		name = req.Name
	}
	filter := &sub.FilterRule
	if req.FilterRule != nil {
		filter = req.FilterRule
	}
	enabled := sub.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if err := s.DB.UpdateSublevel(id, name, filter, enabled); err != nil {
		jsonError(w, "update failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w)
}

func (s *Server) handleDeleteSublevel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.DB.DeleteSublevel(id); err != nil {
		jsonError(w, "delete failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w)
}

func (s *Server) handleRotateKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	newKey, err := s.DB.RotateSublevelKey(id)
	if err != nil {
		jsonError(w, "rotate failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"api_key": newKey})
}

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	ilinkProvider "github.com/openilink/openilink-hub/internal/provider/ilink"
	"github.com/openilink/openilink-hub/internal/auth"
	"github.com/openilink/openilink-hub/internal/database"
	"github.com/openilink/openilink-hub/internal/provider"
)

func (s *Server) handleListBots(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	bots, err := s.DB.ListBotsByUser(userID)
	if err != nil {
		jsonError(w, "list failed", http.StatusInternalServerError)
		return
	}

	type botResp struct {
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Provider  string          `json:"provider"`
		Status    string          `json:"status"`
		MsgCount  int64           `json:"msg_count"`
		CreatedAt int64           `json:"created_at"`
		Extra     json.RawMessage `json:"extra,omitempty"`
	}
	var result []botResp
	for _, b := range bots {
		status := b.Status
		if inst, ok := s.BotManager.GetInstance(b.ID); ok {
			status = inst.Status()
		}
		// Extract non-secret info from credentials for display
		extra := extractPublicCredentials(b.Provider, b.Credentials)
		result = append(result, botResp{
			ID: b.ID, Name: b.Name, Provider: b.Provider,
			Status: status, MsgCount: b.MsgCount, CreatedAt: b.CreatedAt,
			Extra: extra,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// extractPublicCredentials returns non-secret info from credentials for the API response.
func extractPublicCredentials(prov string, creds json.RawMessage) json.RawMessage {
	if prov == "ilink" {
		var c struct {
			BotID       string `json:"bot_id"`
			ILinkUserID string `json:"ilink_user_id"`
		}
		json.Unmarshal(creds, &c)
		data, _ := json.Marshal(map[string]string{
			"bot_id":        c.BotID,
			"ilink_user_id": c.ILinkUserID,
		})
		return data
	}
	return nil
}

func (s *Server) handleBindStart(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())

	sessionID, qrURL, err := ilinkProvider.StartBind(r.Context(), userID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"session_id": sessionID,
		"qr_url":     qrURL,
	})
}

func (s *Server) handleBindStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("sessionID")

	ilinkProvider.PendingBinds.Lock()
	entry, ok := ilinkProvider.PendingBinds.M[sessionID]
	ilinkProvider.PendingBinds.Unlock()
	if !ok {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}

	// SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)

	sendEvent := func(event, data string) {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		if flusher != nil {
			flusher.Flush()
		}
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		result, err := ilinkProvider.PollBind(ctx, sessionID)
		if err != nil {
			sendEvent("error", `{"message":"poll failed"}`)
			return
		}

		switch result.Status {
		case "wait":
			sendEvent("status", `{"status":"wait"}`)
		case "scanned":
			sendEvent("status", `{"status":"scanned"}`)
		case "expired":
			j, _ := json.Marshal(map[string]string{"status": "refreshed", "qr_url": result.QRURL})
			sendEvent("status", string(j))
		case "confirmed":
			bot, err := s.DB.CreateBot(entry.UserID, "", "ilink", result.Credentials)
			if err != nil {
				slog.Error("save bot failed", "err", err)
				sendEvent("error", `{"message":"save failed"}`)
				return
			}

			// Auto-create default channel
			var aiCfg *database.AIConfig
			if r.URL.Query().Get("enable_ai") == "true" {
				aiCfg = &database.AIConfig{Enabled: true, Source: "builtin"}
			}
			s.DB.CreateChannel(bot.ID, "默认", "", nil, aiCfg)

			s.BotManager.StartBot(context.Background(), bot)

			j, _ := json.Marshal(map[string]string{"status": "connected", "bot_id": bot.ID})
			sendEvent("status", string(j))
			return
		}
	}
}

func (s *Server) handleReconnect(w http.ResponseWriter, r *http.Request) {
	botID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())

	bot, err := s.DB.GetBot(botID)
	if err != nil || bot.UserID != userID {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	s.BotManager.StartBot(r.Context(), bot)
	jsonOK(w)
}

func (s *Server) handleDeleteBot(w http.ResponseWriter, r *http.Request) {
	botID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())

	bot, err := s.DB.GetBot(botID)
	if err != nil || bot.UserID != userID {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	s.BotManager.StopBot(botID)
	s.DB.DeleteBot(botID)
	jsonOK(w)
}

func (s *Server) handleUpdateBotName(w http.ResponseWriter, r *http.Request) {
	botID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())

	bot, err := s.DB.GetBot(botID)
	if err != nil || bot.UserID != userID {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		jsonError(w, "name required", http.StatusBadRequest)
		return
	}

	if err := s.DB.UpdateBotName(botID, req.Name); err != nil {
		jsonError(w, "update failed", http.StatusInternalServerError)
		return
	}
	jsonOK(w)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	stats, err := s.DB.GetBotStats(userID)
	if err != nil {
		jsonError(w, "stats failed", http.StatusInternalServerError)
		return
	}
	stats.ConnectedWS = s.Hub.ConnectedCount()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleBotSend(w http.ResponseWriter, r *http.Request) {
	botID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())

	bot, err := s.DB.GetBot(botID)
	if err != nil || bot.UserID != userID {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	var req struct {
		Recipient string `json:"recipient"`
		Text      string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
		jsonError(w, "text required", http.StatusBadRequest)
		return
	}

	inst, ok := s.BotManager.GetInstance(botID)
	if !ok {
		jsonError(w, "bot not connected", http.StatusServiceUnavailable)
		return
	}

	clientID, err := inst.Send(r.Context(), provider.OutboundMessage{
		Recipient: req.Recipient,
		Text:      req.Text,
	})
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}

	payload, _ := json.Marshal(map[string]string{"content": req.Text})
	s.DB.SaveMessage(&database.Message{
		BotID:     botID,
		Direction: "outbound",
		Recipient: req.Recipient,
		MsgType:   "text",
		Payload:   payload,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"ok": "true", "client_id": clientID})
}

func (s *Server) handleBotContacts(w http.ResponseWriter, r *http.Request) {
	botID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())

	bot, err := s.DB.GetBot(botID)
	if err != nil || bot.UserID != userID {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}

	contacts, err := s.DB.ListRecentContacts(botID, 100)
	if err != nil {
		jsonError(w, "query failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(contacts)
}

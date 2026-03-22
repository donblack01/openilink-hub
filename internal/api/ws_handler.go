package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/openilink/openilink-hub/internal/database"
	"github.com/openilink/openilink-hub/internal/relay"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	apiKey := r.URL.Query().Get("key")
	if apiKey == "" {
		http.Error(w, `{"error":"api key required"}`, http.StatusUnauthorized)
		return
	}

	sub, err := s.DB.GetSublevelByAPIKey(apiKey)
	if err != nil || !sub.Enabled {
		http.Error(w, `{"error":"invalid or disabled key"}`, http.StatusUnauthorized)
		return
	}

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade failed", "err", err)
		return
	}

	conn := relay.NewConn(sub.ID, sub.BotDBID, ws, s.Hub)
	s.Hub.Register(conn)

	// Send init message with bot info
	botStatus := "disconnected"
	if inst, ok := s.BotManager.GetInstance(sub.BotDBID); ok {
		botStatus = inst.Status()
	}
	conn.Send(relay.NewEnvelope("init", relay.InitData{
		SublevelID:   sub.ID,
		SublevelName: sub.Name,
		BotDBID:      sub.BotDBID,
		BotStatus:    botStatus,
	}))

	// Replay missed messages since last_seq
	if sub.LastSeq > 0 {
		missed, err := s.DB.GetMessagesSince(sub.BotDBID, sub.LastSeq, 100)
		if err == nil && len(missed) > 0 {
			for _, m := range missed {
				env := relay.NewEnvelope("message", relay.MessageData{
					SeqID:      m.ID,
					FromUserID: m.FromUserID,
					Timestamp:  m.CreatedAt * 1000,
					Items:      []relay.MessageItem{{Type: msgTypeStr(m.MessageType), Text: m.Content}},
				})
				conn.Send(env)
			}
			// Update last_seq to the latest replayed message
			_ = s.DB.UpdateSublevelLastSeq(sub.ID, missed[len(missed)-1].ID)
		}
	}

	go conn.WritePump()
	conn.ReadPump() // blocks
}

func msgTypeStr(t int) string {
	switch t {
	case 2:
		return "image"
	case 3:
		return "voice"
	case 4:
		return "file"
	case 5:
		return "video"
	default:
		return "text"
	}
}

// SetupUpstreamHandler creates the handler for messages from sub-level clients.
func (s *Server) SetupUpstreamHandler() relay.UpstreamHandler {
	return func(conn *relay.Conn, env relay.Envelope) {
		switch env.Type {
		case "send_text":
			var data relay.SendTextData
			if err := json.Unmarshal(env.Data, &data); err != nil {
				conn.Send(relay.NewAck(env.ReqID, false, "", "invalid data"))
				return
			}

			inst, ok := s.BotManager.GetInstance(conn.BotDBID)
			if !ok {
				conn.Send(relay.NewAck(env.ReqID, false, "", "bot not connected"))
				return
			}

			clientID, err := inst.SendText(context.Background(), data.ToUserID, data.Text)
			if err != nil {
				conn.Send(relay.NewAck(env.ReqID, false, "", err.Error()))
				return
			}

			// Log outbound
			sublevelID := conn.SublevelID
			s.DB.SaveMessage(&database.Message{
				BotDBID:     conn.BotDBID,
				Direction:   "outbound",
				FromUserID:  "",
				ToUserID:    data.ToUserID,
				MessageType: 1,
				Content:     data.Text,
				SublevelID:  &sublevelID,
			})
			conn.Send(relay.NewAck(env.ReqID, true, clientID, ""))

		default:
			conn.Send(relay.NewEnvelope("error", relay.ErrorData{
				Code: "unknown_type", Message: "unknown message type: " + env.Type,
			}))
		}
	}
}

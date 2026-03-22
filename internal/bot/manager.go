package bot

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	ilink "github.com/openilink/openilink-sdk-go"
	"github.com/openilink/openilink-hub/internal/database"
	"github.com/openilink/openilink-hub/internal/relay"
)

// Manager manages all active bot instances.
type Manager struct {
	mu        sync.RWMutex
	instances map[string]*Instance
	db        *database.DB
	hub       *relay.Hub
}

func NewManager(db *database.DB, hub *relay.Hub) *Manager {
	return &Manager{
		instances: make(map[string]*Instance),
		db:        db,
		hub:       hub,
	}
}

func (m *Manager) StartAll(ctx context.Context) {
	bots, err := m.db.GetAllBots()
	if err != nil {
		slog.Error("failed to load bots", "err", err)
		return
	}
	for _, b := range bots {
		if b.BotToken == "" {
			continue
		}
		if err := m.StartBot(ctx, &b); err != nil {
			slog.Error("failed to start bot", "bot", b.ID, "err", err)
		}
	}
	slog.Info("started all bots", "count", len(bots))
}

func (m *Manager) StartBot(ctx context.Context, bot *database.Bot) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.instances[bot.ID]; ok {
		old.Stop()
	}
	inst := NewInstance(bot)
	inst.Start(ctx, m.db, m.onInbound, m.onStatusChange)
	m.instances[bot.ID] = inst
	slog.Info("bot started", "bot", bot.ID, "ilink_bot_id", bot.BotID)
	return nil
}

func (m *Manager) StopBot(botDBID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if inst, ok := m.instances[botDBID]; ok {
		inst.Stop()
		delete(m.instances, botDBID)
	}
}

func (m *Manager) GetInstance(botDBID string) (*Instance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst, ok := m.instances[botDBID]
	return inst, ok
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, inst := range m.instances {
		inst.Stop()
	}
	m.instances = make(map[string]*Instance)
}

// onStatusChange broadcasts bot status to all sublevels.
func (m *Manager) onStatusChange(inst *Instance, status string) {
	env := relay.NewEnvelope("bot_status", relay.BotStatusData{
		BotID:  inst.DBID,
		Status: status,
	})
	m.hub.Broadcast(inst.DBID, env)
}

// onInbound routes an inbound message with filtering.
func (m *Manager) onInbound(inst *Instance, msg ilink.WeixinMessage) {
	text := ilink.ExtractText(&msg)
	fromUser := msg.FromUserID

	// Determine message type and content for storage
	content := text
	msgType := 1
	for _, item := range msg.ItemList {
		switch item.Type {
		case ilink.ItemImage:
			msgType = 2
			if content == "" {
				content = "[image]"
			}
		case ilink.ItemVoice:
			msgType = 3
			if item.VoiceItem != nil && item.VoiceItem.Text != "" {
				content = item.VoiceItem.Text
			} else if content == "" {
				content = "[voice]"
			}
		case ilink.ItemFile:
			msgType = 4
			if item.FileItem != nil {
				content = item.FileItem.FileName
			}
		case ilink.ItemVideo:
			msgType = 5
			if content == "" {
				content = "[video]"
			}
		}
	}

	// Save to DB and get sequence ID
	dbMsg := &database.Message{
		BotDBID:     inst.DBID,
		Direction:   "inbound",
		FromUserID:  fromUser,
		MessageType: msgType,
		Content:     content,
	}
	seqID, _ := m.db.SaveMessage(dbMsg)
	_ = m.db.IncrBotMsgCount(inst.DBID)

	// Build relay envelope
	var items []relay.MessageItem
	for _, item := range msg.ItemList {
		switch item.Type {
		case ilink.ItemText:
			if item.TextItem != nil {
				items = append(items, relay.MessageItem{Type: "text", Text: item.TextItem.Text})
			}
		case ilink.ItemImage:
			items = append(items, relay.MessageItem{Type: "image"})
		case ilink.ItemVoice:
			mi := relay.MessageItem{Type: "voice"}
			if item.VoiceItem != nil {
				mi.Text = item.VoiceItem.Text
			}
			items = append(items, mi)
		case ilink.ItemFile:
			mi := relay.MessageItem{Type: "file"}
			if item.FileItem != nil {
				mi.FileName = item.FileItem.FileName
			}
			items = append(items, mi)
		case ilink.ItemVideo:
			items = append(items, relay.MessageItem{Type: "video"})
		}
	}

	env := relay.NewEnvelope("message", relay.MessageData{
		SeqID:        seqID,
		MessageID:    msg.MessageID,
		FromUserID:   fromUser,
		Timestamp:    msg.CreateTimeMs,
		Items:        items,
		ContextToken: msg.ContextToken,
		SessionID:    msg.SessionID,
	})

	// Load sublevels and filter
	subs, err := m.db.ListSublevelsByBot(inst.DBID)
	if err != nil {
		slog.Error("load sublevels failed", "bot", inst.DBID, "err", err)
		return
	}

	for _, sub := range subs {
		if !matchFilter(sub.FilterRule, fromUser, text, msgType) {
			continue
		}
		m.hub.SendTo(sub.ID, env)
		_ = m.db.UpdateSublevelLastSeq(sub.ID, seqID)
	}
}

// matchFilter checks if a message passes the sublevel's filter rule.
func matchFilter(rule database.FilterRule, fromUser, text string, msgType int) bool {
	// User filter
	if len(rule.UserIDs) > 0 {
		found := false
		for _, uid := range rule.UserIDs {
			if uid == fromUser {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Message type filter
	if len(rule.MessageTypes) > 0 {
		found := false
		for _, mt := range rule.MessageTypes {
			if mt == msgType {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Keyword filter (any keyword match in text)
	if len(rule.Keywords) > 0 {
		found := false
		lower := strings.ToLower(text)
		for _, kw := range rule.Keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

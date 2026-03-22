package database

import (
	"time"

	"github.com/google/uuid"
)

type Bot struct {
	ID          string  `json:"id"`
	UserID      string  `json:"user_id"`
	Name        string  `json:"name"`
	BotID       string  `json:"bot_id"`
	BotToken    string  `json:"-"`
	BaseURL     string  `json:"base_url"`
	ILinkUserID string  `json:"ilink_user_id"`
	SyncBuf     string  `json:"-"`
	Status      string  `json:"status"`
	MsgCount    int64   `json:"msg_count"`
	LastMsgAt   *int64  `json:"last_msg_at,omitempty"`
	CreatedAt   int64   `json:"created_at"`
	UpdatedAt   int64   `json:"updated_at"`
}

const botSelectCols = `id, user_id, name, bot_id, bot_token, base_url, ilink_user_id,
	sync_buf, status, msg_count, EXTRACT(EPOCH FROM last_msg_at)::BIGINT,
	EXTRACT(EPOCH FROM created_at)::BIGINT, EXTRACT(EPOCH FROM updated_at)::BIGINT`

func scanBot(scanner interface{ Scan(...any) error }) (*Bot, error) {
	b := &Bot{}
	err := scanner.Scan(&b.ID, &b.UserID, &b.Name, &b.BotID, &b.BotToken, &b.BaseURL,
		&b.ILinkUserID, &b.SyncBuf, &b.Status, &b.MsgCount, &b.LastMsgAt,
		&b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (db *DB) CreateBot(userID, name, botID, botToken, baseURL, ilinkUserID string) (*Bot, error) {
	id := uuid.New().String()
	if name == "" {
		suffix := botID
		if len(suffix) > 8 {
			suffix = suffix[:8]
		}
		name = "Bot-" + suffix
	}
	_, err := db.Exec(
		`INSERT INTO bots (id, user_id, name, bot_id, bot_token, base_url, ilink_user_id, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'connected')`,
		id, userID, name, botID, botToken, baseURL, ilinkUserID,
	)
	if err != nil {
		return nil, err
	}
	return &Bot{ID: id, UserID: userID, Name: name, BotID: botID, BotToken: botToken,
		BaseURL: baseURL, ILinkUserID: ilinkUserID, Status: "connected"}, nil
}

func (db *DB) ListBotsByUser(userID string) ([]Bot, error) {
	rows, err := db.Query("SELECT "+botSelectCols+" FROM bots WHERE user_id = $1 ORDER BY created_at", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bots []Bot
	for rows.Next() {
		b, err := scanBot(rows)
		if err != nil {
			return nil, err
		}
		bots = append(bots, *b)
	}
	return bots, rows.Err()
}

func (db *DB) GetBot(id string) (*Bot, error) {
	return scanBot(db.QueryRow("SELECT "+botSelectCols+" FROM bots WHERE id = $1", id))
}

func (db *DB) GetAllBots() ([]Bot, error) {
	rows, err := db.Query("SELECT " + botSelectCols + " FROM bots")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bots []Bot
	for rows.Next() {
		b, err := scanBot(rows)
		if err != nil {
			return nil, err
		}
		bots = append(bots, *b)
	}
	return bots, rows.Err()
}

func (db *DB) UpdateBotName(id, name string) error {
	_, err := db.Exec("UPDATE bots SET name = $1, updated_at = NOW() WHERE id = $2", name, id)
	return err
}

func (db *DB) UpdateBotStatus(id, status string) error {
	_, err := db.Exec("UPDATE bots SET status = $1, updated_at = NOW() WHERE id = $2", status, id)
	return err
}

func (db *DB) UpdateBotSyncBuf(id, syncBuf string) error {
	_, err := db.Exec("UPDATE bots SET sync_buf = $1, updated_at = NOW() WHERE id = $2", syncBuf, id)
	return err
}

func (db *DB) IncrBotMsgCount(id string) error {
	_, err := db.Exec("UPDATE bots SET msg_count = msg_count + 1, last_msg_at = NOW(), updated_at = NOW() WHERE id = $1", id)
	return err
}

func (db *DB) DeleteBot(id string) error {
	_, err := db.Exec("DELETE FROM bots WHERE id = $1", id)
	return err
}

func (db *DB) CountBotsByUser(userID string) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM bots WHERE user_id = $1", userID).Scan(&count)
	return count, err
}

func (db *DB) CountSublevelsByBot(botDBID string) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sublevels WHERE bot_db_id = $1", botDBID).Scan(&count)
	return count, err
}

// BotStats returns aggregated stats for a user's bots.
type BotStats struct {
	TotalBots      int   `json:"total_bots"`
	OnlineBots     int   `json:"online_bots"`
	TotalSublevels int   `json:"total_sublevels"`
	TotalMessages  int64 `json:"total_messages"`
	ConnectedWS    int   `json:"connected_ws"` // set by API layer
}

func (db *DB) GetBotStats(userID string) (*BotStats, error) {
	s := &BotStats{}
	err := db.QueryRow(`
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'connected'),
			COALESCE((SELECT COUNT(*) FROM sublevels WHERE user_id = $1), 0),
			COALESCE(SUM(msg_count), 0)
		FROM bots WHERE user_id = $1`, userID,
	).Scan(&s.TotalBots, &s.OnlineBots, &s.TotalSublevels, &s.TotalMessages)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// RecentContact tracks WeChat users that have communicated through a bot.
type RecentContact struct {
	UserID    string `json:"user_id"`
	LastMsgAt int64  `json:"last_msg_at"`
	MsgCount  int    `json:"msg_count"`
}

func (db *DB) ListRecentContacts(botDBID string, limit int) ([]RecentContact, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.Query(`
		SELECT from_user_id, EXTRACT(EPOCH FROM MAX(created_at))::BIGINT, COUNT(*)
		FROM messages WHERE bot_db_id = $1 AND direction = 'inbound' AND from_user_id != ''
		GROUP BY from_user_id ORDER BY MAX(created_at) DESC LIMIT $2`,
		botDBID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var contacts []RecentContact
	for rows.Next() {
		var c RecentContact
		if err := rows.Scan(&c.UserID, &c.LastMsgAt, &c.MsgCount); err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, rows.Err()
}

// GetMessagesSince retrieves messages after a given sequence ID for replay.
func (db *DB) GetMessagesSince(botDBID string, afterSeq int64, limit int) ([]Message, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.Query(`
		SELECT id, bot_db_id, direction, from_user_id, message_type, content, sublevel_id,
		       EXTRACT(EPOCH FROM created_at)::BIGINT
		FROM messages WHERE bot_db_id = $1 AND id > $2 ORDER BY id ASC LIMIT $3`,
		botDBID, afterSeq, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.BotDBID, &m.Direction, &m.FromUserID, &m.MessageType, &m.Content, &m.SublevelID, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// LastMessageTime returns the time of the last message received by any bot for a user.
func (db *DB) LastActivityAt(userID string) *time.Time {
	var t *time.Time
	db.QueryRow(`
		SELECT MAX(last_msg_at) FROM bots WHERE user_id = $1`, userID,
	).Scan(&t)
	return t
}

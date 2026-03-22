package database

type Message struct {
	ID          int64   `json:"id"`
	BotDBID     string  `json:"bot_db_id"`
	Direction   string  `json:"direction"`
	FromUserID  string  `json:"from_user_id"`
	ToUserID    string  `json:"to_user_id,omitempty"`
	MessageType int     `json:"message_type"`
	Content     string  `json:"content"`
	SublevelID  *string `json:"sublevel_id,omitempty"`
	CreatedAt   int64   `json:"created_at"`
}

func (db *DB) SaveMessage(m *Message) (int64, error) {
	var id int64
	err := db.QueryRow(`
		INSERT INTO messages (bot_db_id, direction, from_user_id, to_user_id, message_type, content, sublevel_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		m.BotDBID, m.Direction, m.FromUserID, m.ToUserID, m.MessageType, m.Content, m.SublevelID,
	).Scan(&id)
	return id, err
}

func (db *DB) ListMessages(botDBID string, limit int, beforeID int64) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rows interface {
		Close() error
		Next() bool
		Scan(...any) error
		Err() error
	}
	var err error
	if beforeID > 0 {
		rows, err = db.Query(`
			SELECT id, bot_db_id, direction, from_user_id, to_user_id, message_type, content, sublevel_id,
			       EXTRACT(EPOCH FROM created_at)::BIGINT
			FROM messages WHERE bot_db_id = $1 AND id < $2 ORDER BY id DESC LIMIT $3`,
			botDBID, beforeID, limit,
		)
	} else {
		rows, err = db.Query(`
			SELECT id, bot_db_id, direction, from_user_id, to_user_id, message_type, content, sublevel_id,
			       EXTRACT(EPOCH FROM created_at)::BIGINT
			FROM messages WHERE bot_db_id = $1 ORDER BY id DESC LIMIT $2`,
			botDBID, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.BotDBID, &m.Direction, &m.FromUserID, &m.ToUserID,
			&m.MessageType, &m.Content, &m.SublevelID, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (db *DB) ListMessagesByUser(botDBID, fromUserID string, limit int) ([]Message, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.Query(`
		SELECT id, bot_db_id, direction, from_user_id, to_user_id, message_type, content, sublevel_id,
		       EXTRACT(EPOCH FROM created_at)::BIGINT
		FROM messages WHERE bot_db_id = $1 AND (from_user_id = $2 OR to_user_id = $2)
		ORDER BY id DESC LIMIT $3`,
		botDBID, fromUserID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.BotDBID, &m.Direction, &m.FromUserID, &m.ToUserID,
			&m.MessageType, &m.Content, &m.SublevelID, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (db *DB) PruneMessages(maxAgeDays int) (int64, error) {
	result, err := db.Exec("DELETE FROM messages WHERE created_at < NOW() - INTERVAL '1 day' * $1", maxAgeDays)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

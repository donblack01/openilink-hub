package database

type Message struct {
	ID          int64
	BotDBID     string
	Direction   string // "inbound" or "outbound"
	ILinkUserID string
	MessageType int
	Content     string
	SublevelID  *string
	CreatedAt   int64
}

func (db *DB) SaveMessage(botDBID, direction, ilinkUserID string, msgType int, content string, sublevelID *string) error {
	_, err := db.Exec(
		"INSERT INTO messages (bot_db_id, direction, ilink_user_id, message_type, content, sublevel_id) VALUES ($1, $2, $3, $4, $5, $6)",
		botDBID, direction, ilinkUserID, msgType, content, sublevelID,
	)
	return err
}

func (db *DB) ListMessages(botDBID string, limit int, beforeID int64) ([]Message, error) {
	var rows interface {
		Close() error
		Next() bool
		Scan(...any) error
		Err() error
	}
	var err error

	if beforeID > 0 {
		rows, err = db.Query(
			`SELECT id, bot_db_id, direction, ilink_user_id, message_type, content, sublevel_id,
			        EXTRACT(EPOCH FROM created_at)::BIGINT
			 FROM messages WHERE bot_db_id = $1 AND id < $2 ORDER BY id DESC LIMIT $3`,
			botDBID, beforeID, limit,
		)
	} else {
		rows, err = db.Query(
			`SELECT id, bot_db_id, direction, ilink_user_id, message_type, content, sublevel_id,
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
		if err := rows.Scan(&m.ID, &m.BotDBID, &m.Direction, &m.ILinkUserID, &m.MessageType, &m.Content, &m.SublevelID, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

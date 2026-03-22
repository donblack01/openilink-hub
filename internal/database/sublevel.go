package database

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"
)

type FilterRule struct {
	UserIDs      []string `json:"user_ids,omitempty"`      // empty = all users
	Keywords     []string `json:"keywords,omitempty"`       // empty = all messages
	MessageTypes []int    `json:"message_types,omitempty"`  // 1=text,2=image,...; empty = all
}

type Sublevel struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	BotDBID    string     `json:"bot_db_id"`
	Name       string     `json:"name"`
	APIKey     string     `json:"api_key"`
	FilterRule FilterRule `json:"filter_rule"`
	Enabled    bool       `json:"enabled"`
	LastSeq    int64      `json:"last_seq"`
	CreatedAt  int64      `json:"created_at"`
	UpdatedAt  int64      `json:"updated_at"`
}

func generateAPIKey() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

const sublevelSelectCols = `id, user_id, bot_db_id, name, api_key, filter_rule, enabled, last_seq,
	EXTRACT(EPOCH FROM created_at)::BIGINT, EXTRACT(EPOCH FROM updated_at)::BIGINT`

func scanSublevel(scanner interface{ Scan(...any) error }) (*Sublevel, error) {
	s := &Sublevel{}
	var filterJSON []byte
	err := scanner.Scan(&s.ID, &s.UserID, &s.BotDBID, &s.Name, &s.APIKey,
		&filterJSON, &s.Enabled, &s.LastSeq, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(filterJSON, &s.FilterRule)
	return s, nil
}

func (db *DB) CreateSublevel(userID, botDBID, name string, filter *FilterRule) (*Sublevel, error) {
	id := uuid.New().String()
	apiKey := generateAPIKey()
	if filter == nil {
		filter = &FilterRule{}
	}
	filterJSON, _ := json.Marshal(filter)
	_, err := db.Exec(
		"INSERT INTO sublevels (id, user_id, bot_db_id, name, api_key, filter_rule) VALUES ($1, $2, $3, $4, $5, $6)",
		id, userID, botDBID, name, apiKey, filterJSON,
	)
	if err != nil {
		return nil, err
	}
	return &Sublevel{ID: id, UserID: userID, BotDBID: botDBID, Name: name, APIKey: apiKey,
		FilterRule: *filter, Enabled: true}, nil
}

func (db *DB) ListSublevelsByUser(userID string) ([]Sublevel, error) {
	rows, err := db.Query("SELECT "+sublevelSelectCols+" FROM sublevels WHERE user_id = $1 ORDER BY created_at", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subs []Sublevel
	for rows.Next() {
		s, err := scanSublevel(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, *s)
	}
	return subs, rows.Err()
}

func (db *DB) ListSublevelsByBot(botDBID string) ([]Sublevel, error) {
	rows, err := db.Query("SELECT "+sublevelSelectCols+" FROM sublevels WHERE bot_db_id = $1 AND enabled = TRUE", botDBID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subs []Sublevel
	for rows.Next() {
		s, err := scanSublevel(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, *s)
	}
	return subs, rows.Err()
}

func (db *DB) GetSublevelByAPIKey(apiKey string) (*Sublevel, error) {
	return scanSublevel(db.QueryRow("SELECT "+sublevelSelectCols+" FROM sublevels WHERE api_key = $1", apiKey))
}

func (db *DB) GetSublevel(id string) (*Sublevel, error) {
	return scanSublevel(db.QueryRow("SELECT "+sublevelSelectCols+" FROM sublevels WHERE id = $1", id))
}

func (db *DB) UpdateSublevel(id, name string, filter *FilterRule, enabled bool) error {
	filterJSON, _ := json.Marshal(filter)
	_, err := db.Exec(
		"UPDATE sublevels SET name = $1, filter_rule = $2, enabled = $3, updated_at = NOW() WHERE id = $4",
		name, filterJSON, enabled, id,
	)
	return err
}

func (db *DB) DeleteSublevel(id string) error {
	_, err := db.Exec("DELETE FROM sublevels WHERE id = $1", id)
	return err
}

func (db *DB) RotateSublevelKey(id string) (string, error) {
	newKey := generateAPIKey()
	_, err := db.Exec("UPDATE sublevels SET api_key = $1, updated_at = NOW() WHERE id = $2", newKey, id)
	return newKey, err
}

func (db *DB) UpdateSublevelLastSeq(sublevelID string, seq int64) error {
	_, err := db.Exec("UPDATE sublevels SET last_seq = $1, updated_at = NOW() WHERE id = $2", seq, sublevelID)
	return err
}

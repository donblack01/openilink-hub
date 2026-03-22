package database

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/google/uuid"
)

type Sublevel struct {
	ID         string
	UserID     string
	BotDBID    string
	Name       string
	APIKey     string
	FilterRule string
	Enabled    bool
	CreatedAt  int64
	UpdatedAt  int64
}

func generateAPIKey() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

const sublevelSelectCols = `id, user_id, bot_db_id, name, api_key, filter_rule, enabled,
	EXTRACT(EPOCH FROM created_at)::BIGINT, EXTRACT(EPOCH FROM updated_at)::BIGINT`

func scanSublevel(scanner interface{ Scan(...any) error }) (*Sublevel, error) {
	s := &Sublevel{}
	err := scanner.Scan(&s.ID, &s.UserID, &s.BotDBID, &s.Name, &s.APIKey, &s.FilterRule, &s.Enabled, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (db *DB) CreateSublevel(userID, botDBID, name string) (*Sublevel, error) {
	id := uuid.New().String()
	apiKey := generateAPIKey()
	_, err := db.Exec(
		"INSERT INTO sublevels (id, user_id, bot_db_id, name, api_key) VALUES ($1, $2, $3, $4, $5)",
		id, userID, botDBID, name, apiKey,
	)
	if err != nil {
		return nil, err
	}
	return &Sublevel{ID: id, UserID: userID, BotDBID: botDBID, Name: name, APIKey: apiKey, Enabled: true}, nil
}

func (db *DB) ListSublevelsByUser(userID string) ([]Sublevel, error) {
	rows, err := db.Query("SELECT "+sublevelSelectCols+" FROM sublevels WHERE user_id = $1", userID)
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

func (db *DB) DeleteSublevel(id string) error {
	_, err := db.Exec("DELETE FROM sublevels WHERE id = $1", id)
	return err
}

func (db *DB) RotateSublevelKey(id string) (string, error) {
	newKey := generateAPIKey()
	_, err := db.Exec("UPDATE sublevels SET api_key = $1, updated_at = NOW() WHERE id = $2", newKey, id)
	return newKey, err
}

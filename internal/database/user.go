package database

import "github.com/google/uuid"

type User struct {
	ID          string
	Username    string
	DisplayName string
	CreatedAt   int64
}

func (db *DB) CreateUser(username, displayName string) (*User, error) {
	id := uuid.New().String()
	_, err := db.Exec("INSERT INTO users (id, username, display_name) VALUES ($1, $2, $3)", id, username, displayName)
	if err != nil {
		return nil, err
	}
	return &User{ID: id, Username: username, DisplayName: displayName}, nil
}

func (db *DB) GetUserByID(id string) (*User, error) {
	u := &User{}
	err := db.QueryRow("SELECT id, username, display_name, EXTRACT(EPOCH FROM created_at)::BIGINT FROM users WHERE id = $1", id).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (db *DB) GetUserByUsername(username string) (*User, error) {
	u := &User{}
	err := db.QueryRow("SELECT id, username, display_name, EXTRACT(EPOCH FROM created_at)::BIGINT FROM users WHERE username = $1", username).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

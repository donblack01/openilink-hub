package database

type Credential struct {
	ID              string
	UserID          string
	PublicKey       []byte
	AttestationType string
	Transport       string // JSON array
	SignCount       uint32
	CreatedAt       int64
}

func (db *DB) SaveCredential(c *Credential) error {
	_, err := db.Exec(
		"INSERT INTO credentials (id, user_id, public_key, attestation_type, transport, sign_count) VALUES ($1, $2, $3, $4, $5, $6)",
		c.ID, c.UserID, c.PublicKey, c.AttestationType, c.Transport, c.SignCount,
	)
	return err
}

func (db *DB) GetCredentialsByUserID(userID string) ([]Credential, error) {
	rows, err := db.Query(
		"SELECT id, user_id, public_key, attestation_type, transport, sign_count, EXTRACT(EPOCH FROM created_at)::BIGINT FROM credentials WHERE user_id = $1",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []Credential
	for rows.Next() {
		var c Credential
		if err := rows.Scan(&c.ID, &c.UserID, &c.PublicKey, &c.AttestationType, &c.Transport, &c.SignCount, &c.CreatedAt); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

func (db *DB) UpdateCredentialSignCount(id string, signCount uint32) error {
	_, err := db.Exec("UPDATE credentials SET sign_count = $1 WHERE id = $2", signCount, id)
	return err
}

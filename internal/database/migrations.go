package database

const schema = `
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    email         TEXT NOT NULL DEFAULT '',
    display_name  TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL DEFAULT 'member',
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email) WHERE email != '';

CREATE TABLE IF NOT EXISTS credentials (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    public_key       BYTEA NOT NULL,
    attestation_type TEXT NOT NULL DEFAULT '',
    transport        TEXT NOT NULL DEFAULT '[]',
    sign_count       INTEGER NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_credentials_user ON credentials(user_id);

CREATE TABLE IF NOT EXISTS sessions (
    token      TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);

CREATE TABLE IF NOT EXISTS bots (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name          TEXT NOT NULL DEFAULT '',
    bot_id        TEXT NOT NULL,
    bot_token     TEXT NOT NULL,
    base_url      TEXT NOT NULL DEFAULT '',
    ilink_user_id TEXT NOT NULL DEFAULT '',
    sync_buf      TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'disconnected',
    msg_count     BIGINT NOT NULL DEFAULT 0,
    last_msg_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_bots_user ON bots(user_id);

CREATE TABLE IF NOT EXISTS sublevels (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bot_db_id   TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    api_key     TEXT NOT NULL UNIQUE,
    filter_rule JSONB NOT NULL DEFAULT '{}',
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    last_seq    BIGINT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sublevels_bot ON sublevels(bot_db_id);

CREATE TABLE IF NOT EXISTS messages (
    id            BIGSERIAL PRIMARY KEY,
    bot_db_id     TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    direction     TEXT NOT NULL,
    from_user_id  TEXT NOT NULL DEFAULT '',
    to_user_id    TEXT NOT NULL DEFAULT '',
    message_type  INTEGER NOT NULL DEFAULT 1,
    content       TEXT NOT NULL,
    sublevel_id   TEXT DEFAULT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_messages_bot ON messages(bot_db_id, id);
CREATE INDEX IF NOT EXISTS idx_messages_from ON messages(bot_db_id, from_user_id);

-- Migration: add columns if missing (idempotent)
DO $$ BEGIN
    ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT NOT NULL DEFAULT '';
    ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT NOT NULL DEFAULT '';
    ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'member';
    ALTER TABLE users ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
    ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
    ALTER TABLE bots ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
    ALTER TABLE bots ADD COLUMN IF NOT EXISTS msg_count BIGINT NOT NULL DEFAULT 0;
    ALTER TABLE bots ADD COLUMN IF NOT EXISTS last_msg_at TIMESTAMPTZ;
    ALTER TABLE sublevels ADD COLUMN IF NOT EXISTS last_seq BIGINT NOT NULL DEFAULT 0;
    -- Rename ilink_user_id to from_user_id/to_user_id in messages
    -- (skip if already migrated)
EXCEPTION WHEN others THEN NULL;
END $$;
`

CREATE TABLE admin_user (
    singleton INTEGER PRIMARY KEY NOT NULL CHECK (singleton = 1),
    id TEXT NOT NULL UNIQUE,
    password_verifier TEXT NOT NULL,
    password_changed_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE sessions (
    id_hash BLOB PRIMARY KEY NOT NULL CHECK (length(id_hash) = 32),
    csrf_secret_hash BLOB NOT NULL CHECK (length(csrf_secret_hash) = 32),
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT
);

CREATE INDEX sessions_expiration_idx ON sessions (expires_at);
CREATE INDEX sessions_revocation_idx ON sessions (revoked_at);

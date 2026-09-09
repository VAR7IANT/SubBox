CREATE TABLE nodes (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL,
    protocol TEXT NOT NULL CHECK (
        protocol IN ('shadowsocks', 'hysteria2', 'tuic', 'vless_reality', 'anytls')
    ),
    host TEXT NOT NULL,
    listen_port INTEGER NOT NULL CHECK (
        typeof(listen_port) = 'integer' AND listen_port BETWEEN 1 AND 65535
    ),
    public_port INTEGER NOT NULL CHECK (
        typeof(public_port) = 'integer' AND public_port BETWEEN 1 AND 65535
    ),
    enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    credentials_ciphertext BLOB,
    credentials_version INTEGER NOT NULL DEFAULT 0 CHECK (credentials_version >= 0),
    protocol_config TEXT NOT NULL DEFAULT '{}',
    protocol_config_version INTEGER NOT NULL DEFAULT 1 CHECK (protocol_config_version >= 1),
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE subscriptions (
    id TEXT PRIMARY KEY NOT NULL,
    token_hash BLOB NOT NULL UNIQUE CHECK (length(token_hash) > 0),
    token_ciphertext BLOB NOT NULL CHECK (length(token_ciphertext) > 0),
    enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    content_ciphertext BLOB,
    content_revision INTEGER NOT NULL DEFAULT 0 CHECK (content_revision >= 0),
    content_updated_at TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE settings (
    id INTEGER PRIMARY KEY NOT NULL CHECK (id = 1),
    public_base_url TEXT,
    ui_language TEXT NOT NULL DEFAULT 'zh-CN',
    config_adopted_at TEXT,
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE runtime_state (
    id INTEGER PRIMARY KEY NOT NULL CHECK (id = 1),
    config_generation INTEGER NOT NULL DEFAULT 0 CHECK (config_generation >= 0),
    live_config_hash TEXT,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO settings (id) VALUES (1);
INSERT INTO runtime_state (id) VALUES (1);

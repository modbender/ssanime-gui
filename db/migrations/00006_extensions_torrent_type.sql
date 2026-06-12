-- +goose Up
-- +goose StatementBegin
PRAGMA foreign_keys = OFF;
CREATE TABLE extensions_new (
    id          INTEGER PRIMARY KEY,
    uuid        TEXT    NOT NULL UNIQUE,
    repo_id     INTEGER REFERENCES extension_repos (id) ON DELETE SET NULL,
    ext_id      TEXT    NOT NULL UNIQUE,
    name        TEXT    NOT NULL,
    type        TEXT    NOT NULL CHECK (type IN ('torrent', 'manga', 'online-streaming')),
    lang        TEXT    NOT NULL DEFAULT 'js',
    version     TEXT,
    source_url  TEXT,
    payload     TEXT,
    enabled     INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    is_builtin  INTEGER NOT NULL DEFAULT 0 CHECK (is_builtin IN (0, 1)),
    settings    TEXT,
    nsfw        INTEGER NOT NULL DEFAULT 0 CHECK (nsfw IN (0, 1)),
    icon        TEXT,
    added_at    INTEGER NOT NULL DEFAULT (unixepoch()),
    modified_at INTEGER NOT NULL DEFAULT (unixepoch())
);
INSERT INTO extensions_new (id, uuid, repo_id, ext_id, name, type, lang, version, source_url, payload, enabled, is_builtin, settings, added_at, modified_at)
    SELECT id, uuid, repo_id, ext_id, name,
           CASE WHEN type = 'anime-torrent' THEN 'torrent' ELSE type END,
           lang, version, source_url, payload, enabled, is_builtin, settings, added_at, modified_at
    FROM extensions;
DROP TABLE extensions;
ALTER TABLE extensions_new RENAME TO extensions;
CREATE INDEX idx_extensions_repo ON extensions (repo_id);
CREATE INDEX idx_extensions_enabled ON extensions (enabled);
CREATE INDEX idx_extensions_type ON extensions (type);
PRAGMA foreign_keys = ON;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE settings ADD COLUMN setup_completed INTEGER NOT NULL DEFAULT 0 CHECK (setup_completed IN (0, 1));
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE settings ADD COLUMN show_nsfw INTEGER NOT NULL DEFAULT 0 CHECK (show_nsfw IN (0, 1));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE settings DROP COLUMN show_nsfw;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE settings DROP COLUMN setup_completed;
-- +goose StatementEnd

-- +goose StatementBegin
PRAGMA foreign_keys = OFF;
CREATE TABLE extensions_old (
    id          INTEGER PRIMARY KEY,
    uuid        TEXT    NOT NULL UNIQUE,
    repo_id     INTEGER REFERENCES extension_repos (id) ON DELETE SET NULL,
    ext_id      TEXT    NOT NULL UNIQUE,
    name        TEXT    NOT NULL,
    type        TEXT    NOT NULL CHECK (type IN ('anime-torrent', 'manga', 'online-streaming')),
    lang        TEXT    NOT NULL DEFAULT 'js',
    version     TEXT,
    source_url  TEXT,
    payload     TEXT,
    enabled     INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    is_builtin  INTEGER NOT NULL DEFAULT 0 CHECK (is_builtin IN (0, 1)),
    settings    TEXT,
    added_at    INTEGER NOT NULL DEFAULT (unixepoch()),
    modified_at INTEGER NOT NULL DEFAULT (unixepoch())
);
INSERT INTO extensions_old (id, uuid, repo_id, ext_id, name, type, lang, version, source_url, payload, enabled, is_builtin, settings, added_at, modified_at)
    SELECT id, uuid, repo_id, ext_id, name,
           CASE WHEN type = 'torrent' THEN 'anime-torrent' ELSE type END,
           lang, version, source_url, payload, enabled, is_builtin, settings, added_at, modified_at
    FROM extensions;
DROP TABLE extensions;
ALTER TABLE extensions_old RENAME TO extensions;
CREATE INDEX idx_extensions_repo ON extensions (repo_id);
CREATE INDEX idx_extensions_enabled ON extensions (enabled);
CREATE INDEX idx_extensions_type ON extensions (type);
PRAGMA foreign_keys = ON;
-- +goose StatementEnd

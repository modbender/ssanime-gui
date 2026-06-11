-- +goose Up
-- +goose StatementBegin
-- Durable cache for the merged AniList + ani.zip series-detail payload. One row
-- per AniList id; payload is the full AnilistDetail JSON served verbatim to the
-- frontend. fetched_at (unix seconds) drives the 24h freshness check. This is the
-- persistence layer for per-episode metadata (no episodes-table expansion).
CREATE TABLE anilist_detail_cache (
    anilist_id INTEGER PRIMARY KEY,
    payload    TEXT    NOT NULL,
    fetched_at INTEGER NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE anilist_detail_cache;
-- +goose StatementEnd

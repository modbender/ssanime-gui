-- +goose Up
-- +goose StatementBegin
-- Watch status: the AniList-style state that solely drives polling. Supersedes the
-- old user_status override layer. Values: 'watching' (polled), 'on_hold' and
-- 'dropped' (never polled). 'completed' is NOT stored here; it stays derived
-- (finished airing + all episodes archived). Named watch_status because the series
-- table already has a status column holding the raw AniList media status; the API
-- surfaces this column as the JSON field "status". The legacy user_status column is
-- left in place (unused by the gate now) to keep this migration reversible and
-- low-risk; subscribed stays pinned to 1 for every subscribed row (unsubscribe
-- deletes the row).
ALTER TABLE series ADD COLUMN watch_status TEXT NOT NULL DEFAULT 'watching'
    CHECK (watch_status IN ('watching', 'on_hold', 'dropped'));
-- +goose StatementEnd

-- +goose StatementBegin
-- Backfill from the old user_status override: paused -> on_hold, dropped -> dropped,
-- everything else (NULL / subscribed-active) -> watching (the column default).
UPDATE series SET watch_status = 'on_hold' WHERE user_status = 'paused';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE series SET watch_status = 'dropped' WHERE user_status = 'dropped';
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_series_watch_status ON series (watch_status);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX idx_series_watch_status;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE series DROP COLUMN watch_status;
-- +goose StatementEnd

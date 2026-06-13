-- +goose Up
-- +goose StatementBegin
-- Drop the legacy user_status override column. It is fully superseded by
-- watch_status (added in 00008): subscription is the subscribed flag, and the
-- AniList-style poll gate reads watch_status. Nothing in the app reads
-- user_status anymore.
ALTER TABLE series DROP COLUMN user_status;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE series ADD COLUMN user_status TEXT;
-- +goose StatementEnd

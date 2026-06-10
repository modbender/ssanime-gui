-- +goose Up
-- +goose StatementBegin
-- Unix seconds of the last AniList metadata refresh; NULL = never refreshed (so a
-- freshly-created series sorts first for the background refresher).
ALTER TABLE series ADD COLUMN metadata_refreshed_at INTEGER;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE series DROP COLUMN metadata_refreshed_at;
-- +goose StatementEnd

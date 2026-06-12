-- +goose Up
-- +goose StatementBegin
ALTER TABLE episodes ADD COLUMN source_cleaned_at INTEGER;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE episodes DROP COLUMN source_cleaned_at;
-- +goose StatementEnd

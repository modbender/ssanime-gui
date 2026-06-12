-- +goose Up
-- +goose StatementBegin
ALTER TABLE series ADD COLUMN cover_color TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE series DROP COLUMN cover_color;
-- +goose StatementEnd

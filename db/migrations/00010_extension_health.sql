-- +goose Up
-- +goose StatementBegin
ALTER TABLE extensions ADD COLUMN healthy INTEGER;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE extensions ADD COLUMN health_error TEXT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE extensions ADD COLUMN health_checked_at INTEGER;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE extensions DROP COLUMN health_checked_at;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE extensions DROP COLUMN health_error;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE extensions DROP COLUMN healthy;
-- +goose StatementEnd

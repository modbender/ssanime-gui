-- +goose Up
-- +goose StatementBegin
-- The trusted release group this series is locked to, set from the first
-- downloaded episode's group and thereafter preferred by the poller. For each new
-- episode the poller prefers this group and only falls back to the OTHER trusted
-- group after (air date + 24h). NULL means not yet locked — the poller takes the
-- best trusted release and the enqueue sets the lock. Manual downloads also set it
-- when empty but are otherwise exempt (a manual grab may use any group).
ALTER TABLE series ADD COLUMN locked_release_group TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE series DROP COLUMN locked_release_group;
-- +goose StatementEnd

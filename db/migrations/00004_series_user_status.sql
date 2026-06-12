-- +goose Up
-- +goose StatementBegin
-- Manual status override layer: NULL = fully automatic (derivedStatus governs and
-- background automation runs); 'paused'/'dropped' = user override that displays
-- that status and makes the series' feed dormant (no background poll/download).
-- No status change ever deletes files; this is purely the automation gate.
ALTER TABLE series ADD COLUMN user_status TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE series DROP COLUMN user_status;
-- +goose StatementEnd

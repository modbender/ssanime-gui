-- +goose Up
-- +goose StatementBegin
-- Backfill watermark: the highest episode number that had already aired at the
-- moment the series was first subscribed. The poller only auto-downloads
-- episodes ABOVE this floor, so subscribing mid-season doesn't pull the whole
-- back-catalogue. NULL (or 0) means "no floor" — the poller may take everything,
-- which is correct for a brand-new not-yet-aired series subscribed before any
-- episode existed. Manual/explicit downloads ignore this column entirely.
ALTER TABLE series ADD COLUMN backfill_from_episode INTEGER;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE series DROP COLUMN backfill_from_episode;
-- +goose StatementEnd

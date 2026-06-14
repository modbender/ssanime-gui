-- +goose Up
-- +goose StatementBegin
-- The user-configurable trusted release-group allowlist, stored as a JSON array of
-- strings ordered by preference (earlier = better). The poller and on-demand source
-- search rank/filter releases against this list. An explicitly empty array ('[]')
-- means "no trust filter" — selection falls back to best-available rather than
-- dropping every release. The default mirrors the original hardcoded allowlist.
ALTER TABLE settings ADD COLUMN trusted_release_groups TEXT NOT NULL DEFAULT '["SubsPlease","Erai-raws"]';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE settings DROP COLUMN trusted_release_groups;
-- +goose StatementEnd

-- +goose Up
-- +goose StatementBegin
-- Inheritable subtitle burn-in toggle (NULL = inherit). When set, the selected
-- subtitle track is rendered into the video and soft sub/attachment copy is
-- dropped. Implicitly forced for MP4 + a non-text source sub at encode time.
ALTER TABLE encode_profiles ADD COLUMN burn_subs INTEGER CHECK (burn_subs IN (0, 1));
-- +goose StatementEnd

-- +goose StatementBegin
-- Inheritable per-track language selection (NULL = inherit; resolved to the
-- wildcard/passthrough sentinel when the whole chain is NULL). A JSON array of
-- normalized language codes (e.g. ["en","ja"]) selects specific tracks; an
-- explicit NULL means All (MKV) / Default-track (MP4).
ALTER TABLE encode_profiles ADD COLUMN audio_languages TEXT;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE encode_profiles ADD COLUMN subtitle_languages TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE encode_profiles DROP COLUMN burn_subs;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE encode_profiles DROP COLUMN audio_languages;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE encode_profiles DROP COLUMN subtitle_languages;
-- +goose StatementEnd

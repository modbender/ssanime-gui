-- +goose Up
-- +goose StatementBegin
-- Inheritable bit-depth knob (NULL = inherit; resolved child->parent). 10 selects
-- 10-bit output (yuv420p10le), which nearly eliminates x265-introduced gradient
-- banding on flat anime backgrounds; unset falls back to 8-bit (current behavior).
ALTER TABLE encode_profiles ADD COLUMN bit_depth INTEGER;
-- +goose StatementEnd

-- +goose StatementBegin
-- Inheritable deband filter toggle (NULL = inherit). When set, ffmpeg's deband
-- filter runs at output resolution as a banding-mitigation fallback.
ALTER TABLE encode_profiles ADD COLUMN deband INTEGER CHECK (deband IN (0, 1));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE encode_profiles DROP COLUMN bit_depth;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE encode_profiles DROP COLUMN deband;
-- +goose StatementEnd

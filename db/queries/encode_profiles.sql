-- name: GetEncodeProfile :one
SELECT * FROM encode_profiles WHERE id = ?;

-- name: GetEncodeProfileByUUID :one
SELECT * FROM encode_profiles WHERE uuid = ?;

-- name: GetEncodeProfileByName :one
SELECT * FROM encode_profiles WHERE name = ?;

-- name: ListEncodeProfiles :many
SELECT * FROM encode_profiles ORDER BY builtin DESC, name ASC;

-- name: ListBuiltinEncodeProfiles :many
SELECT * FROM encode_profiles WHERE builtin = 1 ORDER BY name ASC;

-- ResolveProfileChain walks parent_id from the given profile to the root,
-- ordered child-first; Go COALESCEs each knob field across the chain.
-- name: ResolveProfileChain :many
WITH RECURSIVE chain(
    id, uuid, name, builtin, parent_id, codec, crf, preset, smartblur,
    deinterlace, deblock, psy_rd, psy_rdoq, aq_strength, aq_mode, scale,
    audio, container, x265_params, bit_depth, deband, output_resolutions,
    added_at, modified_at, depth
) AS (
    SELECT
        ep.id, ep.uuid, ep.name, ep.builtin, ep.parent_id, ep.codec, ep.crf,
        ep.preset, ep.smartblur, ep.deinterlace, ep.deblock, ep.psy_rd,
        ep.psy_rdoq, ep.aq_strength, ep.aq_mode, ep.scale, ep.audio,
        ep.container, ep.x265_params, ep.bit_depth, ep.deband,
        ep.output_resolutions, ep.added_at, ep.modified_at, 0 AS depth
    FROM encode_profiles ep
    WHERE ep.id = ?
    UNION ALL
    SELECT
        p.id, p.uuid, p.name, p.builtin, p.parent_id, p.codec, p.crf,
        p.preset, p.smartblur, p.deinterlace, p.deblock, p.psy_rd,
        p.psy_rdoq, p.aq_strength, p.aq_mode, p.scale, p.audio,
        p.container, p.x265_params, p.bit_depth, p.deband,
        p.output_resolutions, p.added_at, p.modified_at, c.depth + 1
    FROM encode_profiles p
    JOIN chain c ON p.id = c.parent_id
)
SELECT
    id, uuid, name, builtin, parent_id, codec, crf, preset, smartblur,
    deinterlace, deblock, psy_rd, psy_rdoq, aq_strength, aq_mode, scale,
    audio, container, x265_params, bit_depth, deband, output_resolutions,
    added_at, modified_at
FROM chain ORDER BY depth ASC;

-- name: CreateEncodeProfile :one
INSERT INTO encode_profiles (
    uuid, name, builtin, parent_id, codec, crf, preset, smartblur,
    deinterlace, deblock, psy_rd, psy_rdoq, aq_strength, aq_mode, scale,
    audio, container, x265_params, bit_depth, deband, output_resolutions
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: UpdateEncodeProfile :one
UPDATE encode_profiles SET
    name = ?, parent_id = ?, codec = ?, crf = ?, preset = ?, smartblur = ?,
    deinterlace = ?, deblock = ?, psy_rd = ?, psy_rdoq = ?, aq_strength = ?,
    aq_mode = ?, scale = ?, audio = ?, container = ?, x265_params = ?,
    bit_depth = ?, deband = ?, output_resolutions = ?, modified_at = unixepoch()
WHERE id = ? AND builtin = 0
RETURNING *;

-- name: DeleteEncodeProfile :exec
DELETE FROM encode_profiles WHERE id = ? AND builtin = 0;

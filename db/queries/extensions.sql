-- name: GetExtensionRepo :one
SELECT * FROM extension_repos WHERE id = ?;

-- name: GetExtensionRepoByURL :one
SELECT * FROM extension_repos WHERE url = ?;

-- name: ListExtensionRepos :many
SELECT * FROM extension_repos ORDER BY name ASC;

-- name: ListEnabledExtensionRepos :many
SELECT * FROM extension_repos WHERE enabled = 1 ORDER BY name ASC;

-- name: CreateExtensionRepo :one
INSERT INTO extension_repos (uuid, name, url, enabled)
VALUES (?, ?, ?, ?)
RETURNING *;

-- name: MarkExtensionRepoSynced :exec
UPDATE extension_repos SET last_synced_at = unixepoch() WHERE id = ?;

-- name: SetExtensionRepoEnabled :exec
UPDATE extension_repos SET enabled = ? WHERE id = ?;

-- name: DeleteExtensionRepo :exec
DELETE FROM extension_repos WHERE id = ?;

-- name: GetExtension :one
SELECT * FROM extensions WHERE id = ?;

-- name: GetExtensionByExtID :one
SELECT * FROM extensions WHERE ext_id = ?;

-- name: ListExtensions :many
SELECT * FROM extensions ORDER BY name ASC;

-- name: ListEnabledExtensionsByType :many
SELECT * FROM extensions WHERE enabled = 1 AND type = ? ORDER BY name ASC;

-- name: ListExtensionsByRepo :many
SELECT * FROM extensions WHERE repo_id = ? ORDER BY name ASC;

-- name: CreateExtension :one
INSERT INTO extensions (
    uuid, repo_id, ext_id, name, type, lang, version, source_url,
    payload, enabled, is_builtin, settings, nsfw, icon
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: UpsertExtensionByExtID :one
INSERT INTO extensions (
    uuid, repo_id, ext_id, name, type, lang, version, source_url,
    payload, enabled, is_builtin, settings, nsfw, icon
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
ON CONFLICT (ext_id) DO UPDATE SET
    repo_id = excluded.repo_id,
    name = excluded.name,
    type = excluded.type,
    lang = excluded.lang,
    version = excluded.version,
    source_url = excluded.source_url,
    payload = excluded.payload,
    settings = excluded.settings,
    nsfw = excluded.nsfw,
    icon = excluded.icon,
    modified_at = unixepoch()
RETURNING *;

-- name: SetExtensionEnabled :exec
UPDATE extensions SET enabled = ?, modified_at = unixepoch() WHERE id = ?;

-- name: UpdateExtensionSettings :exec
UPDATE extensions SET settings = ?, modified_at = unixepoch() WHERE id = ?;

-- name: UpdateExtensionHealth :exec
UPDATE extensions
SET healthy = ?, health_error = ?, health_checked_at = COALESCE(?, unixepoch())
WHERE ext_id = ?;

-- name: DeleteExtension :exec
DELETE FROM extensions WHERE id = ? AND is_builtin = 0;

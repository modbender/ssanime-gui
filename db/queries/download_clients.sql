-- name: GetDownloadClient :one
SELECT * FROM download_clients WHERE id = ?;

-- name: GetDefaultDownloadClient :one
SELECT * FROM download_clients WHERE is_default = 1 LIMIT 1;

-- name: ListDownloadClients :many
SELECT * FROM download_clients ORDER BY is_default DESC, name ASC;

-- name: ListEnabledDownloadClients :many
SELECT * FROM download_clients WHERE enabled = 1 ORDER BY is_default DESC, name ASC;

-- name: CreateDownloadClient :one
INSERT INTO download_clients (
    uuid, kind, name, host, port, username, password, enabled, is_default, settings
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: UpdateDownloadClient :one
UPDATE download_clients SET
    kind = ?, name = ?, host = ?, port = ?, username = ?, password = ?,
    enabled = ?, settings = ?
WHERE id = ?
RETURNING *;

-- ClearDefaultDownloadClients resets all defaults before setting a new one.
-- name: ClearDefaultDownloadClients :exec
UPDATE download_clients SET is_default = 0;

-- name: SetDefaultDownloadClient :exec
UPDATE download_clients SET is_default = 1 WHERE id = ?;

-- name: DeleteDownloadClient :exec
DELETE FROM download_clients WHERE id = ?;

-- name: GetScreenshot :one
SELECT * FROM screenshots WHERE id = ?;

-- name: ListScreenshotsByEpisode :many
SELECT * FROM screenshots WHERE episode_id = ? ORDER BY ordinal ASC;

-- name: ListScreenshotsBySeries :many
SELECT * FROM screenshots WHERE series_id = ? ORDER BY episode_id ASC, ordinal ASC;

-- name: CreateScreenshot :one
INSERT INTO screenshots (
    uuid, episode_id, series_id, path, caption, ordinal
) VALUES (
    ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: DeleteScreenshotsByEpisode :exec
DELETE FROM screenshots WHERE episode_id = ?;

-- name: DeleteScreenshot :exec
DELETE FROM screenshots WHERE id = ?;

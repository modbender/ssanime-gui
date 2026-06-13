-- name: GetEpisode :one
SELECT * FROM episodes WHERE id = ?;

-- GetEpisodeWithSeries returns an episode joined with its series title so the
-- detail/queue DTOs can show the series name without a second query.
-- name: GetEpisodeWithSeries :one
SELECT sqlc.embed(episodes), series.title AS series_title
FROM episodes
JOIN series ON series.id = episodes.series_id
WHERE episodes.id = ?;

-- name: GetEpisodeByUUID :one
SELECT * FROM episodes WHERE uuid = ?;

-- name: ListEpisodesBySeries :many
SELECT * FROM episodes WHERE series_id = ? ORDER BY episode_no ASC NULLS LAST, added_at ASC;

-- name: ListEpisodesByStatus :many
SELECT * FROM episodes WHERE status = ? ORDER BY added_at ASC;

-- name: CountEpisodesBySeries :one
SELECT COUNT(*) FROM episodes WHERE series_id = ?;

-- name: ListQueuedEpisodes :many
SELECT * FROM episodes WHERE status = 'queued' ORDER BY added_at ASC;

-- name: CreateEpisode :one
INSERT INTO episodes (
    uuid, series_id, title, episode_no, source_kind, source_url, magnet,
    release_group, resolution, subtype, uncensored, bluray, published_at,
    status, profile_id
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: SetEpisodeStatus :exec
UPDATE episodes SET status = ?, modified_at = unixepoch() WHERE id = ?;

-- name: SetEpisodeError :exec
UPDATE episodes SET status = 'error', error_message = ?, modified_at = unixepoch() WHERE id = ?;

-- name: MarkEpisodeDownloading :exec
UPDATE episodes SET status = 'downloading', modified_at = unixepoch() WHERE id = ?;

-- name: MarkEpisodeDownloaded :exec
UPDATE episodes SET
    status = 'downloaded', source_path = ?, source_size = ?,
    downloaded_at = unixepoch(), modified_at = unixepoch()
WHERE id = ?;

-- name: MarkEpisodeEncoding :exec
UPDATE episodes SET
    status = 'encoding', encoded_params_snapshot = ?, modified_at = unixepoch()
WHERE id = ?;

-- name: MarkEpisodeEncoded :exec
UPDATE episodes SET
    status = 'encoded', encoded_at = unixepoch(), modified_at = unixepoch()
WHERE id = ?;

-- name: MarkEpisodeArchived :exec
UPDATE episodes SET status = 'archived', modified_at = unixepoch() WHERE id = ?;

-- name: IncrementEpisodeRetry :exec
UPDATE episodes SET retry_count = retry_count + 1, modified_at = unixepoch() WHERE id = ?;

-- SetEpisodeSourcePath updates source_path after the original is moved/cleaned up
-- so the row points at the new location (move) or NULL (delete).
-- name: SetEpisodeSourcePath :exec
UPDATE episodes SET source_path = ?, modified_at = unixepoch() WHERE id = ?;

-- name: ClearEpisodeError :exec
UPDATE episodes SET error_message = NULL, modified_at = unixepoch() WHERE id = ?;

-- MarkEpisodeSourceCleaned stamps the unix time the source files were deleted
-- under cleanup_policy=delete, so the UI can show "source cleaned up".
-- name: MarkEpisodeSourceCleaned :exec
UPDATE episodes SET source_cleaned_at = ?, modified_at = unixepoch() WHERE id = ?;

-- Crash recovery: orphaned in-flight statuses reset to the last durable state.
-- name: ResetOrphanedDownloadingEpisodes :exec
UPDATE episodes SET status = 'queued', modified_at = unixepoch() WHERE status = 'downloading';

-- name: ResetOrphanedEncodingEpisodes :exec
UPDATE episodes SET status = 'downloaded', modified_at = unixepoch() WHERE status = 'encoding';

-- name: DeleteEpisode :exec
DELETE FROM episodes WHERE id = ?;

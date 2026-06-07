-- name: GetEncodedOutput :one
SELECT * FROM encoded_outputs WHERE id = ?;

-- name: ListEncodedOutputsByEpisode :many
SELECT * FROM encoded_outputs WHERE episode_id = ? ORDER BY resolution DESC;

-- name: ListEncodedOutputsByStatus :many
SELECT * FROM encoded_outputs WHERE status = ? ORDER BY added_at ASC;

-- CountUnarchivedOutputs reports how many of an episode's outputs are not yet
-- archived; 0 means the episode is fully archived (cleanup trigger).
-- name: CountUnarchivedOutputs :one
SELECT COUNT(*) FROM encoded_outputs WHERE episode_id = ? AND status != 'archived';

-- CountErroredOutputs reports failed outputs for an episode; >0 keeps the original.
-- name: CountErroredOutputs :one
SELECT COUNT(*) FROM encoded_outputs WHERE episode_id = ? AND status = 'error';

-- name: CreateEncodedOutput :one
INSERT INTO encoded_outputs (
    uuid, episode_id, resolution, profile_id, encoded_params_snapshot, status
) VALUES (
    ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: SetEncodedOutputStatus :exec
UPDATE encoded_outputs SET status = ?, modified_at = unixepoch() WHERE id = ?;

-- SetEncodedOutputSnapshot records the resolved x265 args used for an output
-- (reproducibility), set once the arg builder has produced them.
-- name: SetEncodedOutputSnapshot :exec
UPDATE encoded_outputs SET
    encoded_params_snapshot = ?, modified_at = unixepoch()
WHERE id = ?;

-- name: SetEncodedOutputError :exec
UPDATE encoded_outputs SET
    status = 'error', error_message = ?, modified_at = unixepoch()
WHERE id = ?;

-- name: MarkEncodedOutputEncoded :exec
UPDATE encoded_outputs SET
    status = 'encoded', encoded_path = ?, encoded_size = ?,
    encoded_at = unixepoch(), modified_at = unixepoch()
WHERE id = ?;

-- name: MarkEncodedOutputArchived :exec
UPDATE encoded_outputs SET status = 'archived', modified_at = unixepoch() WHERE id = ?;

-- Crash recovery: in-flight encode/thumbnail work resets to queued.
-- name: ResetOrphanedEncodedOutputs :exec
UPDATE encoded_outputs SET status = 'queued', modified_at = unixepoch()
WHERE status IN ('encoding', 'thumbnailing');

-- name: DeleteEncodedOutput :exec
DELETE FROM encoded_outputs WHERE id = ?;

-- name: GetFeed :one
SELECT * FROM feeds WHERE id = ?;

-- name: GetFeedByURL :one
SELECT * FROM feeds WHERE url = ?;

-- name: ListFeeds :many
SELECT * FROM feeds ORDER BY added_at ASC;

-- name: ListFeedsBySeries :many
SELECT * FROM feeds WHERE series_id = ? ORDER BY added_at ASC;

-- ListFeedsDueForPoll returns enabled feeds whose series is subscribed and whose
-- derived status still permits polling (not completed/cancelled/not_aired), and
-- whose interval has elapsed. The completed/up_to_date split is computed in Go from
-- airing_status + archive counts; this query enforces the cheap, durable filters.
-- name: ListFeedsDueForPoll :many
SELECT f.*
FROM feeds f
JOIN series s ON s.id = f.series_id
WHERE f.enabled = 1
  AND s.subscribed = 1
  AND (s.airing_status IS NULL OR s.airing_status NOT IN ('CANCELLED', 'NOT_YET_RELEASED'))
  AND (
        f.last_checked_at IS NULL
     OR f.last_checked_at + f.interval_seconds <= sqlc.arg(now)
  )
ORDER BY f.last_checked_at ASC NULLS FIRST;

-- name: CreateFeed :one
INSERT INTO feeds (
    uuid, series_id, type, site, url, quality, subtype, deinterlace,
    uncensored, bluray, title_regex, extra_tags, interval_seconds,
    offset_seconds, seen_cache, enabled
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: UpdateFeed :one
UPDATE feeds SET
    type = ?, site = ?, url = ?, quality = ?, subtype = ?, deinterlace = ?,
    uncensored = ?, bluray = ?, title_regex = ?, extra_tags = ?,
    interval_seconds = ?, offset_seconds = ?, enabled = ?, modified_at = unixepoch()
WHERE id = ?
RETURNING *;

-- name: MarkFeedChecked :exec
UPDATE feeds SET
    last_checked_at = sqlc.arg(now), seen_cache = sqlc.arg(seen_cache),
    error_message = NULL, modified_at = unixepoch()
WHERE id = sqlc.arg(id);

-- name: MarkFeedError :exec
UPDATE feeds SET
    last_checked_at = sqlc.arg(now), error_message = sqlc.arg(error_message),
    modified_at = unixepoch()
WHERE id = sqlc.arg(id);

-- name: SetFeedEnabled :exec
UPDATE feeds SET enabled = ?, modified_at = unixepoch() WHERE id = ?;

-- name: DeleteFeed :exec
DELETE FROM feeds WHERE id = ?;

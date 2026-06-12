-- name: GetAnilistDetailCache :one
SELECT * FROM anilist_detail_cache WHERE anilist_id = ?;

-- name: UpsertAnilistDetailCache :exec
INSERT INTO anilist_detail_cache (anilist_id, payload, fetched_at)
VALUES (?, ?, ?)
ON CONFLICT (anilist_id) DO UPDATE SET
    payload = excluded.payload,
    fetched_at = excluded.fetched_at;

-- name: DeleteAnilistDetailCache :exec
DELETE FROM anilist_detail_cache WHERE anilist_id = ?;

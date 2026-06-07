-- name: GetSeries :one
SELECT * FROM series WHERE id = ?;

-- name: GetSeriesByUUID :one
SELECT * FROM series WHERE uuid = ?;

-- name: GetSeriesByTitle :one
SELECT * FROM series WHERE title = ?;

-- name: GetSeriesByAnilistID :one
SELECT * FROM series WHERE anilist_id = ?;

-- name: ListSeries :many
SELECT * FROM series ORDER BY title ASC;

-- name: ListSubscribedSeries :many
SELECT * FROM series WHERE subscribed = 1 ORDER BY title ASC;

-- name: ListFavoriteSeries :many
SELECT * FROM series WHERE favorite = 1 ORDER BY title ASC;

-- ListSeriesWithProgress joins archived-episode counts for the Library grid and
-- derived-status computation (archived = every selected output archived).
-- name: ListSeriesWithProgress :many
SELECT
    s.*,
    COUNT(DISTINCT e.id) AS episode_total,
    COUNT(DISTINCT CASE
        WHEN e.status = 'archived' THEN e.id
    END) AS episode_archived,
    COALESCE(SUM(e.source_size), 0) AS source_bytes_total,
    COALESCE((
        SELECT SUM(eo.encoded_size)
        FROM encoded_outputs eo
        JOIN episodes e2 ON e2.id = eo.episode_id
        WHERE e2.series_id = s.id AND eo.status = 'archived'
    ), 0) AS encoded_bytes_total
FROM series s
LEFT JOIN episodes e ON e.series_id = s.id
GROUP BY s.id
ORDER BY s.title ASC;

-- name: CreateSeries :one
INSERT INTO series (
    uuid, title, feed_title, alt_titles, season_number, subscribed, favorite,
    airing_status, poster_path, poster_portrait, default_profile_id,
    anilist_id, mal_id, romaji_title, english_title, format, status,
    episode_count, synonyms, cover_image_url, banner_image_url, cover_color, season, season_year
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: UpdateSeries :one
UPDATE series SET
    title = ?, feed_title = ?, alt_titles = ?, season_number = ?,
    subscribed = ?, favorite = ?, airing_status = ?, poster_path = ?,
    poster_portrait = ?, default_profile_id = ?, anilist_id = ?, mal_id = ?,
    romaji_title = ?, english_title = ?, format = ?, status = ?,
    episode_count = ?, synonyms = ?, cover_image_url = ?, banner_image_url = ?,
    cover_color = ?, season = ?, season_year = ?, modified_at = unixepoch()
WHERE id = ?
RETURNING *;

-- name: SetSeriesSubscribed :exec
UPDATE series SET subscribed = ?, modified_at = unixepoch() WHERE id = ?;

-- name: SetSeriesFavorite :exec
UPDATE series SET favorite = ?, modified_at = unixepoch() WHERE id = ?;

-- name: SetSeriesAiringStatus :exec
UPDATE series SET airing_status = ?, modified_at = unixepoch() WHERE id = ?;

-- name: DeleteSeries :exec
DELETE FROM series WHERE id = ?;

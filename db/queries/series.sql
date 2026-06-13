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
    episode_count, synonyms, cover_image_url, banner_image_url, cover_color, season, season_year,
    metadata_refreshed_at
) VALUES (
    ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
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

-- ListSeriesForMetadataRefresh returns subscribed, non-finished series whose
-- AniList metadata is stale (never refreshed, or older than the cutoff). NULLs
-- sort first on ASC, so never-refreshed series are picked up before stale ones.
-- name: ListSeriesForMetadataRefresh :many
SELECT * FROM series
WHERE anilist_id IS NOT NULL
  AND subscribed = 1
  AND (airing_status IS NULL OR airing_status NOT IN ('FINISHED', 'CANCELLED'))
  AND (metadata_refreshed_at IS NULL OR metadata_refreshed_at < ?)
ORDER BY metadata_refreshed_at ASC
LIMIT ?;

-- UpdateSeriesMetadata refreshes the volatile AniList-derived columns for one
-- series, preserving user/display fields (title, subscribed, favorite,
-- season_number, default_profile_id, feed_title are never touched). Authoritative
-- scalars (status/airing_status/episode_count/format/season/season_year) are set
-- directly; images, colour, titles and synonyms use COALESCE(NULLIF(@x, ''), col)
-- so a sparse response never blanks a previously-populated column.
-- name: UpdateSeriesMetadata :exec
UPDATE series SET
    status = @status,
    airing_status = @airing_status,
    episode_count = @episode_count,
    format = @format,
    season = @season,
    season_year = @season_year,
    cover_image_url = COALESCE(NULLIF(@cover_image_url, ''), cover_image_url),
    banner_image_url = COALESCE(NULLIF(@banner_image_url, ''), banner_image_url),
    cover_color = COALESCE(NULLIF(@cover_color, ''), cover_color),
    romaji_title = COALESCE(NULLIF(@romaji_title, ''), romaji_title),
    english_title = COALESCE(NULLIF(@english_title, ''), english_title),
    synonyms = COALESCE(NULLIF(@synonyms, ''), synonyms),
    metadata_refreshed_at = @now,
    modified_at = @now
WHERE id = @id;

-- name: SetSeriesSubscribed :exec
UPDATE series SET subscribed = ?, modified_at = unixepoch() WHERE id = ?;

-- name: SetSeriesFavorite :exec
UPDATE series SET favorite = ?, modified_at = unixepoch() WHERE id = ?;

-- name: SetSeriesAiringStatus :exec
UPDATE series SET airing_status = ?, modified_at = unixepoch() WHERE id = ?;

-- SetSeriesWatchStatus sets the watch status that solely drives polling: 'watching'
-- (polled), 'on_hold'/'dropped' (never polled). 'completed' is never stored here.
-- name: SetSeriesWatchStatus :exec
UPDATE series SET watch_status = ?, modified_at = unixepoch() WHERE id = ?;

-- name: DeleteSeries :exec
DELETE FROM series WHERE id = ?;

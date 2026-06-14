-- name: GetSettings :one
SELECT * FROM settings WHERE id = 1;

-- name: SettingsExist :one
SELECT EXISTS (SELECT 1 FROM settings WHERE id = 1) AS present;

-- name: InsertSettings :one
INSERT INTO settings (
    id, download_root, encoded_root, cleanup_policy, processed_dir,
    naming_template, download_backend, default_profile_id,
    concurrency_download, concurrency_encode, ffmpeg_path, ytdlp_path,
    port, doh_enabled, trusted_release_groups
) VALUES (
    1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
)
RETURNING *;

-- name: UpdateSettings :one
UPDATE settings SET
    download_root = ?, encoded_root = ?, cleanup_policy = ?, processed_dir = ?,
    naming_template = ?, download_backend = ?, default_profile_id = ?,
    concurrency_download = ?, concurrency_encode = ?, ffmpeg_path = ?,
    ytdlp_path = ?, port = ?, doh_enabled = ?, setup_completed = ?,
    show_nsfw = ?, trusted_release_groups = ?, modified_at = unixepoch()
WHERE id = 1
RETURNING *;

-- name: SetDefaultProfile :exec
UPDATE settings SET default_profile_id = ?, modified_at = unixepoch() WHERE id = 1;

-- name: SetDownloadBackend :exec
UPDATE settings SET download_backend = ?, modified_at = unixepoch() WHERE id = 1;

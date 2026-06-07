-- +goose Up
-- +goose StatementBegin

-- All timestamps are unix epoch seconds stored as INTEGER (DEFAULT unixepoch()).
-- ids are INTEGER PRIMARY KEY (rowid alias); every table also carries an opaque
-- `uuid TEXT` used for per-item file directories (load-bearing, inherited from automin).

PRAGMA foreign_keys = ON;

-- ---------------------------------------------------------------------------
-- encode_profiles — self-referential inheritance (builtin base + user overrides)
-- Knob columns are NULLABLE: NULL = inherit from parent (resolved child->parent).
-- builtin rows are fully specified and immutable (enforced in app layer).
-- ---------------------------------------------------------------------------
CREATE TABLE encode_profiles (
    id                 INTEGER PRIMARY KEY,
    uuid               TEXT    NOT NULL UNIQUE,
    name               TEXT    NOT NULL UNIQUE,
    builtin            INTEGER NOT NULL DEFAULT 0 CHECK (builtin IN (0, 1)),
    parent_id          INTEGER REFERENCES encode_profiles (id) ON DELETE SET NULL,

    -- Inheritable encode knobs (NULL = inherit from parent chain).
    codec              TEXT,                  -- e.g. 'x265'
    crf                REAL,                  -- x265 CRF is fractional (default 24.2)
    preset             TEXT,                  -- e.g. 'slow'
    smartblur          INTEGER CHECK (smartblur IN (0, 1)),
    deinterlace        INTEGER CHECK (deinterlace IN (0, 1)),
    deblock            TEXT,                  -- e.g. '1,1'
    psy_rd             REAL,
    psy_rdoq           REAL,
    aq_strength        REAL,
    aq_mode            INTEGER,
    scale              INTEGER,               -- target height (1080/720/480) when fixed
    audio              TEXT,                  -- audio handling directive
    container          TEXT,                  -- e.g. 'mkv'
    x265_params        TEXT,                  -- extra raw x265 params
    output_resolutions TEXT,                  -- json int set, e.g. '[1080,720,480]'

    added_at           INTEGER NOT NULL DEFAULT (unixepoch()),
    modified_at        INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX idx_encode_profiles_parent ON encode_profiles (parent_id);
CREATE INDEX idx_encode_profiles_builtin ON encode_profiles (builtin);

-- ---------------------------------------------------------------------------
-- series — one anime entry. automin Anime + EpisodeInfo title bits + AniList enrich.
-- ---------------------------------------------------------------------------
CREATE TABLE series (
    id                 INTEGER PRIMARY KEY,
    uuid               TEXT    NOT NULL UNIQUE,
    title              TEXT    NOT NULL UNIQUE,
    feed_title         TEXT,
    alt_titles         TEXT,                  -- json array
    season_number      INTEGER NOT NULL DEFAULT 1,

    subscribed         INTEGER NOT NULL DEFAULT 0 CHECK (subscribed IN (0, 1)),
    favorite           INTEGER NOT NULL DEFAULT 0 CHECK (favorite IN (0, 1)),

    -- cached AniList airing status; drives derived status + auto-poll.
    airing_status      TEXT CHECK (airing_status IN (
                           'NOT_YET_RELEASED', 'RELEASING', 'FINISHED',
                           'HIATUS', 'CANCELLED'
                       )),

    poster_path        TEXT,
    poster_portrait    INTEGER NOT NULL DEFAULT 1 CHECK (poster_portrait IN (0, 1)),
    default_profile_id INTEGER REFERENCES encode_profiles (id) ON DELETE SET NULL,

    -- AniList metadata (comprehensive-v1).
    anilist_id         INTEGER,
    mal_id             INTEGER,
    romaji_title       TEXT,
    english_title      TEXT,
    format             TEXT,                  -- TV/MOVIE/OVA/ONA/SPECIAL
    status             TEXT,                  -- raw AniList media status
    episode_count      INTEGER,
    synonyms           TEXT,                  -- json array
    cover_image_url    TEXT,
    banner_image_url   TEXT,
    season             TEXT,                  -- WINTER/SPRING/SUMMER/FALL
    season_year        INTEGER,

    added_at           INTEGER NOT NULL DEFAULT (unixepoch()),
    modified_at        INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX idx_series_subscribed ON series (subscribed);
CREATE INDEX idx_series_favorite ON series (favorite);
CREATE INDEX idx_series_airing_status ON series (airing_status);
CREATE INDEX idx_series_anilist_id ON series (anilist_id);
CREATE INDEX idx_series_default_profile ON series (default_profile_id);

-- ---------------------------------------------------------------------------
-- feeds — RSS feeds + scrape watchers merged (automin Feed + SiteWatcher).
-- ---------------------------------------------------------------------------
CREATE TABLE feeds (
    id               INTEGER PRIMARY KEY,
    uuid             TEXT    NOT NULL UNIQUE,
    series_id        INTEGER NOT NULL REFERENCES series (id) ON DELETE CASCADE,
    type             TEXT    NOT NULL CHECK (type IN ('rss', 'scrape')),
    site             TEXT,
    url              TEXT    NOT NULL UNIQUE,

    -- filter rules
    quality          INTEGER,                 -- 1080/720/480, nullable = any
    subtype          TEXT CHECK (subtype IS NULL OR subtype IN ('hardsubs', 'softsubs')),
    deinterlace      INTEGER NOT NULL DEFAULT 0 CHECK (deinterlace IN (0, 1)),
    uncensored       INTEGER NOT NULL DEFAULT 0 CHECK (uncensored IN (0, 1)),
    bluray           INTEGER NOT NULL DEFAULT 0 CHECK (bluray IN (0, 1)),
    title_regex      TEXT,
    extra_tags       TEXT,

    interval_seconds INTEGER NOT NULL DEFAULT 3600,
    offset_seconds   INTEGER NOT NULL DEFAULT 0,
    last_checked_at  INTEGER,
    seen_cache       TEXT,                    -- json: already-seen entry ids

    enabled          INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    error_message    TEXT,

    added_at         INTEGER NOT NULL DEFAULT (unixepoch()),
    modified_at      INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX idx_feeds_series ON feeds (series_id);
CREATE INDEX idx_feeds_enabled ON feeds (enabled);
CREATE INDEX idx_feeds_last_checked ON feeds (last_checked_at);

-- ---------------------------------------------------------------------------
-- episodes — the core entity (source + overall status). Encoded outputs are
-- children in encoded_outputs (encoded_path/encoded_size dropped here).
-- ---------------------------------------------------------------------------
CREATE TABLE episodes (
    id                      INTEGER PRIMARY KEY,
    uuid                    TEXT    NOT NULL UNIQUE,
    series_id               INTEGER NOT NULL REFERENCES series (id) ON DELETE CASCADE,
    title                   TEXT,
    episode_no              INTEGER,            -- NULL = movie/OVA/special

    source_kind             TEXT NOT NULL DEFAULT 'torrent'
                                CHECK (source_kind IN ('torrent', 'direct', 'hls')),
    source_url              TEXT,
    magnet                  TEXT,
    release_group           TEXT,
    resolution              INTEGER,            -- 1080/720/480
    subtype                 TEXT CHECK (subtype IS NULL OR subtype IN ('hardsubs', 'softsubs')),
    uncensored              INTEGER NOT NULL DEFAULT 0 CHECK (uncensored IN (0, 1)),
    bluray                  INTEGER NOT NULL DEFAULT 0 CHECK (bluray IN (0, 1)),
    published_at            INTEGER,

    status                  TEXT NOT NULL DEFAULT 'queued' CHECK (status IN (
                                'queued', 'downloading', 'downloaded', 'encoding',
                                'encoded', 'thumbnailing', 'archived', 'error'
                            )),

    source_path             TEXT,
    source_size             INTEGER,            -- BIGINT
    profile_id              INTEGER REFERENCES encode_profiles (id) ON DELETE SET NULL,
    encoded_params_snapshot TEXT,               -- json: resolved x265 args at encode time

    downloaded_at           INTEGER,
    encoded_at              INTEGER,
    retry_count             INTEGER NOT NULL DEFAULT 0,
    error_message           TEXT,

    added_at                INTEGER NOT NULL DEFAULT (unixepoch()),
    modified_at             INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX idx_episodes_series ON episodes (series_id);
CREATE INDEX idx_episodes_status ON episodes (status);
CREATE INDEX idx_episodes_profile ON episodes (profile_id);
CREATE INDEX idx_episodes_series_episode ON episodes (series_id, episode_no);

-- ---------------------------------------------------------------------------
-- encoded_outputs — one downloaded episode fans out to N encoded files (per res).
-- ---------------------------------------------------------------------------
CREATE TABLE encoded_outputs (
    id                      INTEGER PRIMARY KEY,
    uuid                    TEXT    NOT NULL UNIQUE,
    episode_id              INTEGER NOT NULL REFERENCES episodes (id) ON DELETE CASCADE,
    resolution              INTEGER NOT NULL,   -- 1080/720/480
    profile_id              INTEGER REFERENCES encode_profiles (id) ON DELETE SET NULL,
    encoded_params_snapshot TEXT,               -- json: resolved x265 args used

    status                  TEXT NOT NULL DEFAULT 'queued' CHECK (status IN (
                                'queued', 'encoding', 'encoded',
                                'thumbnailing', 'archived', 'error'
                            )),

    encoded_path            TEXT,
    encoded_size            INTEGER,            -- BIGINT
    error_message           TEXT,
    encoded_at              INTEGER,

    added_at                INTEGER NOT NULL DEFAULT (unixepoch()),
    modified_at             INTEGER NOT NULL DEFAULT (unixepoch()),

    UNIQUE (episode_id, resolution)
);
CREATE INDEX idx_encoded_outputs_episode ON encoded_outputs (episode_id);
CREATE INDEX idx_encoded_outputs_status ON encoded_outputs (status);
CREATE INDEX idx_encoded_outputs_profile ON encoded_outputs (profile_id);

-- ---------------------------------------------------------------------------
-- screenshots — library thumbnails generated post-encode (automin genscr).
-- ---------------------------------------------------------------------------
CREATE TABLE screenshots (
    id         INTEGER PRIMARY KEY,
    uuid       TEXT    NOT NULL UNIQUE,
    episode_id INTEGER NOT NULL REFERENCES episodes (id) ON DELETE CASCADE,
    series_id  INTEGER NOT NULL REFERENCES series (id) ON DELETE CASCADE,
    path       TEXT    NOT NULL,
    caption    TEXT,
    ordinal    INTEGER NOT NULL DEFAULT 0,
    added_at   INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX idx_screenshots_episode ON screenshots (episode_id);
CREATE INDEX idx_screenshots_series ON screenshots (series_id);

-- ---------------------------------------------------------------------------
-- extension_repos — community marketplace sources (raw index.json url).
-- ---------------------------------------------------------------------------
CREATE TABLE extension_repos (
    id             INTEGER PRIMARY KEY,
    uuid           TEXT    NOT NULL UNIQUE,
    name           TEXT    NOT NULL,
    url            TEXT    NOT NULL UNIQUE,
    enabled        INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    last_synced_at INTEGER,
    added_at       INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX idx_extension_repos_enabled ON extension_repos (enabled);

-- ---------------------------------------------------------------------------
-- extensions — installed providers (native builtins have no JS payload).
-- ---------------------------------------------------------------------------
CREATE TABLE extensions (
    id          INTEGER PRIMARY KEY,
    uuid        TEXT    NOT NULL UNIQUE,
    repo_id     INTEGER REFERENCES extension_repos (id) ON DELETE SET NULL,
    ext_id      TEXT    NOT NULL UNIQUE,
    name        TEXT    NOT NULL,
    type        TEXT    NOT NULL CHECK (type IN ('anime-torrent', 'manga', 'online-streaming')),
    lang        TEXT    NOT NULL DEFAULT 'js',
    version     TEXT,
    source_url  TEXT,
    payload     TEXT,                           -- JS source (NULL for native builtins)
    enabled     INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    is_builtin  INTEGER NOT NULL DEFAULT 0 CHECK (is_builtin IN (0, 1)),
    settings    TEXT,                           -- json
    added_at    INTEGER NOT NULL DEFAULT (unixepoch()),
    modified_at INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX idx_extensions_repo ON extensions (repo_id);
CREATE INDEX idx_extensions_enabled ON extensions (enabled);
CREATE INDEX idx_extensions_type ON extensions (type);

-- ---------------------------------------------------------------------------
-- download_clients — embedded anacrolix + external qbit/transmission backends.
-- ---------------------------------------------------------------------------
CREATE TABLE download_clients (
    id         INTEGER PRIMARY KEY,
    uuid       TEXT    NOT NULL UNIQUE,
    kind       TEXT    NOT NULL CHECK (kind IN ('embedded', 'qbittorrent', 'transmission')),
    name       TEXT    NOT NULL,
    host       TEXT,
    port       INTEGER,
    username   TEXT,
    password   TEXT,
    enabled    INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    is_default INTEGER NOT NULL DEFAULT 0 CHECK (is_default IN (0, 1)),
    settings   TEXT,                            -- json
    added_at   INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX idx_download_clients_enabled ON download_clients (enabled);
CREATE INDEX idx_download_clients_default ON download_clients (is_default);

-- ---------------------------------------------------------------------------
-- settings — singleton (id always 1).
-- ---------------------------------------------------------------------------
CREATE TABLE settings (
    id                   INTEGER PRIMARY KEY CHECK (id = 1),
    download_root        TEXT    NOT NULL,
    encoded_root         TEXT    NOT NULL,
    cleanup_policy       TEXT    NOT NULL DEFAULT 'delete'
                             CHECK (cleanup_policy IN ('delete', 'keep', 'move')),
    processed_dir        TEXT,
    naming_template      TEXT    NOT NULL,
    download_backend     INTEGER REFERENCES download_clients (id) ON DELETE SET NULL,
    default_profile_id   INTEGER REFERENCES encode_profiles (id) ON DELETE SET NULL,
    concurrency_download INTEGER NOT NULL DEFAULT 3,
    concurrency_encode   INTEGER NOT NULL DEFAULT 1,
    ffmpeg_path          TEXT,
    ytdlp_path           TEXT,
    port                 INTEGER NOT NULL DEFAULT 4773,
    doh_enabled          INTEGER NOT NULL DEFAULT 1 CHECK (doh_enabled IN (0, 1)),
    added_at             INTEGER NOT NULL DEFAULT (unixepoch()),
    modified_at          INTEGER NOT NULL DEFAULT (unixepoch())
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS download_clients;
DROP TABLE IF EXISTS extensions;
DROP TABLE IF EXISTS extension_repos;
DROP TABLE IF EXISTS screenshots;
DROP TABLE IF EXISTS encoded_outputs;
DROP TABLE IF EXISTS episodes;
DROP TABLE IF EXISTS feeds;
DROP TABLE IF EXISTS series;
DROP TABLE IF EXISTS encode_profiles;
-- +goose StatementEnd

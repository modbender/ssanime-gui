package server

import (
	"encoding/json"

	"github.com/modbender/ssanime-gui/internal/store"
)

// ---- Profile DTO ----

// ProfileResponse is the stable wire shape for encode profiles. It normalises
// the sqlc model: Builtin int64 → is_builtin bool, and OutputResolutions *string
// (stored JSON like "[1080,720]") → output_resolutions []int so callers never
// need to parse the raw string.
type ProfileResponse struct {
	ID                int64    `json:"id"`
	UUID              string   `json:"uuid"`
	Name              string   `json:"name"`
	IsBuiltin         bool     `json:"is_builtin"`
	ParentID          *int64   `json:"parent_id"`
	Codec             *string  `json:"codec"`
	CRF               *float64 `json:"crf"`
	Preset            *string  `json:"preset"`
	Smartblur         *bool    `json:"smartblur"`
	Deinterlace       *bool    `json:"deinterlace"`
	Deblock           *string  `json:"deblock"`
	PsyRD             *float64 `json:"psy_rd"`
	PsyRDOQ           *float64 `json:"psy_rdoq"`
	AQStrength        *float64 `json:"aq_strength"`
	AQMode            *int64   `json:"aq_mode"`
	Scale             *int64   `json:"scale"`
	Audio             *string  `json:"audio"`
	Container         *string  `json:"container"`
	X265Params        *string  `json:"x265_params"`
	OutputResolutions []int    `json:"output_resolutions"`
	AddedAt           int64    `json:"added_at"`
	ModifiedAt        int64    `json:"modified_at"`
}

// toProfileResponse converts a store.EncodeProfile to the stable ProfileResponse
// wire shape. int64 boolean columns (Builtin, Smartblur, Deinterlace) are mapped
// to their Go bool equivalents; the output_resolutions JSON string is parsed into
// []int (nil/invalid → nil, which serialises as JSON null).
func toProfileResponse(p store.EncodeProfile) ProfileResponse {
	var smartblur *bool
	if p.Smartblur != nil {
		v := *p.Smartblur != 0
		smartblur = &v
	}
	var deinterlace *bool
	if p.Deinterlace != nil {
		v := *p.Deinterlace != 0
		deinterlace = &v
	}
	var resolutions []int
	if p.OutputResolutions != nil && *p.OutputResolutions != "" {
		_ = json.Unmarshal([]byte(*p.OutputResolutions), &resolutions)
	}
	return ProfileResponse{
		ID:                p.ID,
		UUID:              p.Uuid,
		Name:              p.Name,
		IsBuiltin:         p.Builtin != 0,
		ParentID:          p.ParentID,
		Codec:             p.Codec,
		CRF:               p.Crf,
		Preset:            p.Preset,
		Smartblur:         smartblur,
		Deinterlace:       deinterlace,
		Deblock:           p.Deblock,
		PsyRD:             p.PsyRd,
		PsyRDOQ:           p.PsyRdoq,
		AQStrength:        p.AqStrength,
		AQMode:            p.AqMode,
		Scale:             p.Scale,
		Audio:             p.Audio,
		Container:         p.Container,
		X265Params:        p.X265Params,
		OutputResolutions: resolutions,
		AddedAt:           p.AddedAt,
		ModifiedAt:        p.ModifiedAt,
	}
}

// ---- Series DTOs ----

// SeriesProgress is the Library-grid row: series + episode counts + space savings.
type SeriesProgress struct {
	ID                int64   `json:"id"`
	UUID              string  `json:"uuid"`
	Title             string  `json:"title"`
	FeedTitle         *string `json:"feed_title"`
	SeasonNumber      int64   `json:"season_number"`
	Subscribed        bool    `json:"subscribed"`
	Favorite          bool    `json:"favorite"`
	AiringStatus      *string `json:"airing_status"`
	Status            string  `json:"status"`
	DerivedStatus     string  `json:"derived_status"`
	PosterPath        *string `json:"poster_path"`
	CoverImageURL     *string `json:"cover_image_url"`
	BannerImageURL    *string `json:"banner_image_url"`
	CoverColor        *string `json:"cover_color"`
	AnilistID         *int64  `json:"anilist_id"`
	RomajiTitle       *string `json:"romaji_title"`
	EnglishTitle      *string `json:"english_title"`
	Format            *string `json:"format"`
	EpisodeCount      *int64  `json:"episode_count"`
	EpisodeTotal      int64   `json:"episode_total"`
	EpisodeArchived   int64   `json:"episode_archived"`
	SourceBytesTotal  int64   `json:"source_bytes_total"`
	EncodedBytesTotal int64   `json:"encoded_bytes_total"`
	SpaceSavedBytes   int64   `json:"space_saved_bytes"`
	AddedAt           int64   `json:"added_at"`
	ModifiedAt        int64   `json:"modified_at"`
}

// SeriesDetail is the series-detail page: series row + episodes with their outputs.
type SeriesDetail struct {
	ID               int64           `json:"id"`
	UUID             string          `json:"uuid"`
	Title            string          `json:"title"`
	FeedTitle        *string         `json:"feed_title"`
	AltTitles        *string         `json:"alt_titles"`
	SeasonNumber     int64           `json:"season_number"`
	Subscribed       bool            `json:"subscribed"`
	Favorite         bool            `json:"favorite"`
	AiringStatus     *string         `json:"airing_status"`
	Status           string          `json:"status"`
	DerivedStatus    string          `json:"derived_status"`
	PosterPath       *string         `json:"poster_path"`
	CoverImageURL    *string         `json:"cover_image_url"`
	BannerImageURL   *string         `json:"banner_image_url"`
	CoverColor       *string         `json:"cover_color"`
	AnilistID        *int64          `json:"anilist_id"`
	RomajiTitle      *string         `json:"romaji_title"`
	EnglishTitle     *string         `json:"english_title"`
	Format           *string         `json:"format"`
	EpisodeCount     *int64          `json:"episode_count"`
	DefaultProfileID *int64          `json:"default_profile_id"`
	Episodes         []EpisodeDetail `json:"episodes"`
	AddedAt          int64           `json:"added_at"`
	ModifiedAt       int64           `json:"modified_at"`
}

// EpisodeDetail is one episode row + its encoded_outputs.
type EpisodeDetail struct {
	ID              int64           `json:"id"`
	UUID            string          `json:"uuid"`
	SeriesID        int64           `json:"series_id"`
	SeriesTitle     string          `json:"series_title"`
	Title           *string         `json:"title"`
	EpisodeNo       *int64          `json:"episode_no"`
	Status          string          `json:"status"`
	Resolution      *int64          `json:"resolution"`
	ReleaseGroup    *string         `json:"release_group"`
	Subtype         *string         `json:"subtype"`
	Uncensored      bool            `json:"uncensored"`
	Bluray          bool            `json:"bluray"`
	SourceSize      *int64          `json:"source_size"`
	SourcePath      *string         `json:"source_path"`
	SourceCleanedAt *int64          `json:"source_cleaned_at"`
	ProfileID       *int64          `json:"profile_id"`
	ErrorMessage    *string         `json:"error_message"`
	RetryCount      int64           `json:"retry_count"`
	PublishedAt     *int64          `json:"published_at"`
	DownloadedAt    *int64          `json:"downloaded_at"`
	EncodedAt       *int64          `json:"encoded_at"`
	Outputs         []OutputSummary `json:"outputs"`
	AddedAt         int64           `json:"added_at"`
	ModifiedAt      int64           `json:"modified_at"`
}

// OutputSummary is one encoded_outputs row for the UI.
type OutputSummary struct {
	ID           int64   `json:"id"`
	UUID         string  `json:"uuid"`
	Resolution   int64   `json:"resolution"`
	Status       string  `json:"status"`
	EncodedPath  *string `json:"encoded_path"`
	EncodedSize  *int64  `json:"encoded_size"`
	ErrorMessage *string `json:"error_message"`
	EncodedAt    *int64  `json:"encoded_at"`
}

// ---- Series request bodies ----

// CreateSeriesRequest adds a series by AniList ID or free-text title.
type CreateSeriesRequest struct {
	AnilistID    *int64  `json:"anilist_id"`
	Title        *string `json:"title"`
	SeasonNumber *int64  `json:"season_number"`
	ProfileID    *int64  `json:"default_profile_id"`
}

// PatchSeriesRequest allows partial updates to mutable series fields.
type PatchSeriesRequest struct {
	Subscribed       *bool   `json:"subscribed"`
	Favorite         *bool   `json:"favorite"`
	SeasonNumber     *int64  `json:"season_number"`
	DefaultProfileID *int64  `json:"default_profile_id"`
	AiringStatus     *string `json:"airing_status"`
}

// ---- Encode request bodies ----

// BulkEncodeRequest enqueues a set of episodes for download+encode.
type BulkEncodeRequest struct {
	EpisodeIDs  []int64 `json:"episode_ids"`
	ProfileID   *int64  `json:"profile_id"`
	Resolutions []int   `json:"resolutions"`
}

// ---- Profile DTOs ----

type CreateProfileRequest struct {
	Name              string   `json:"name"`
	ParentID          *int64   `json:"parent_id"`
	Codec             *string  `json:"codec"`
	CRF               *float64 `json:"crf"`
	Preset            *string  `json:"preset"`
	Smartblur         *bool    `json:"smartblur"`
	Deinterlace       *bool    `json:"deinterlace"`
	Deblock           *string  `json:"deblock"`
	PsyRD             *float64 `json:"psy_rd"`
	PsyRDOQ           *float64 `json:"psy_rdoq"`
	AQStrength        *float64 `json:"aq_strength"`
	AQMode            *int64   `json:"aq_mode"`
	Scale             *int64   `json:"scale"`
	Audio             *string  `json:"audio"`
	Container         *string  `json:"container"`
	X265Params        *string  `json:"x265_params"`
	OutputResolutions []int    `json:"output_resolutions"`
}

// PatchProfileRequest shares the same fields as CreateProfileRequest; all fields
// are pointers so nil means "no change".
type PatchProfileRequest = CreateProfileRequest

// ResolvedProfileResponse is the effective profile config after inheritance.
type ResolvedProfileResponse struct {
	ProfileID         int64   `json:"profile_id"`
	Codec             string  `json:"codec"`
	CRF               float64 `json:"crf"`
	Preset            string  `json:"preset"`
	SmartBlur         bool    `json:"smartblur"`
	Deinterlace       bool    `json:"deinterlace"`
	Deblock           string  `json:"deblock"`
	PsyRD             float64 `json:"psy_rd"`
	PsyRDOQ           float64 `json:"psy_rdoq"`
	AQStrength        float64 `json:"aq_strength"`
	AQMode            int     `json:"aq_mode"`
	Audio             string  `json:"audio"`
	Container         string  `json:"container"`
	X265Params        string  `json:"x265_params"`
	OutputResolutions []int   `json:"output_resolutions"`
}

// ---- Settings ----

type PutSettingsRequest struct {
	DownloadRoot        string  `json:"download_root"`
	EncodedRoot         string  `json:"encoded_root"`
	CleanupPolicy       string  `json:"cleanup_policy"`
	ProcessedDir        *string `json:"processed_dir"`
	NamingTemplate      string  `json:"naming_template"`
	DownloadBackend     *int64  `json:"download_backend"`
	DefaultProfileID    *int64  `json:"default_profile_id"`
	ConcurrencyDownload int64   `json:"concurrency_download"`
	ConcurrencyEncode   int64   `json:"concurrency_encode"`
	FfmpegPath          *string `json:"ffmpeg_path"`
	YtdlpPath           *string `json:"ytdlp_path"`
	Port                int64   `json:"port"`
	DohEnabled          bool    `json:"doh_enabled"`
	SetupCompleted      bool    `json:"setup_completed"`
	ShowNsfw            bool    `json:"show_nsfw"`
}

// SettingsResponse is the stable settings wire shape. It serialises the int64
// flag columns (doh_enabled, setup_completed, show_nsfw) as JSON booleans so GET
// and PUT use the same types and a client can round-trip the object unchanged.
type SettingsResponse struct {
	ID                  int64   `json:"id"`
	DownloadRoot        string  `json:"download_root"`
	EncodedRoot         string  `json:"encoded_root"`
	CleanupPolicy       string  `json:"cleanup_policy"`
	ProcessedDir        *string `json:"processed_dir"`
	NamingTemplate      string  `json:"naming_template"`
	DownloadBackend     *int64  `json:"download_backend"`
	DefaultProfileID    *int64  `json:"default_profile_id"`
	ConcurrencyDownload int64   `json:"concurrency_download"`
	ConcurrencyEncode   int64   `json:"concurrency_encode"`
	FfmpegPath          *string `json:"ffmpeg_path"`
	YtdlpPath           *string `json:"ytdlp_path"`
	Port                int64   `json:"port"`
	DohEnabled          bool    `json:"doh_enabled"`
	SetupCompleted      bool    `json:"setup_completed"`
	ShowNsfw            bool    `json:"show_nsfw"`
	AddedAt             int64   `json:"added_at"`
	ModifiedAt          int64   `json:"modified_at"`
}

// toSettingsResponse maps the sqlc Setting row to the bool-flagged wire shape.
func toSettingsResponse(s store.Setting) SettingsResponse {
	return SettingsResponse{
		ID:                  s.ID,
		DownloadRoot:        s.DownloadRoot,
		EncodedRoot:         s.EncodedRoot,
		CleanupPolicy:       s.CleanupPolicy,
		ProcessedDir:        s.ProcessedDir,
		NamingTemplate:      s.NamingTemplate,
		DownloadBackend:     s.DownloadBackend,
		DefaultProfileID:    s.DefaultProfileID,
		ConcurrencyDownload: s.ConcurrencyDownload,
		ConcurrencyEncode:   s.ConcurrencyEncode,
		FfmpegPath:          s.FfmpegPath,
		YtdlpPath:           s.YtdlpPath,
		Port:                s.Port,
		DohEnabled:          s.DohEnabled != 0,
		SetupCompleted:      s.SetupCompleted != 0,
		ShowNsfw:            s.ShowNsfw != 0,
		AddedAt:             s.AddedAt,
		ModifiedAt:          s.ModifiedAt,
	}
}

// ---- Stats ----

type StatsResponse struct {
	SeriesTotal       int64 `json:"series_total"`
	EpisodesArchived  int64 `json:"episodes_archived"`
	SourceBytesTotal  int64 `json:"source_bytes_total"`
	EncodedBytesTotal int64 `json:"encoded_bytes_total"`
	SpaceSavedBytes   int64 `json:"space_saved_bytes"`
}

// ---- Queue ----

type QueueSnapshot struct {
	Downloading []EpisodeDetail `json:"downloading"`
	Encoding    []EpisodeDetail `json:"encoding"`
}

// ---- Search ----

type AnilistSearchResult struct {
	ID           int      `json:"id"`
	IDMal        *int     `json:"idMal"`
	RomajiTitle  string   `json:"romaji_title"`
	EnglishTitle string   `json:"english_title"`
	Format       string   `json:"format"`
	Status       string   `json:"status"`
	EpisodeCount int      `json:"episode_count"`
	CoverImage   string   `json:"cover_image"`
	BannerImage  string   `json:"banner_image"`
	Season       string   `json:"season"`
	SeasonYear   int      `json:"season_year"`
	Synonyms     []string `json:"synonyms"`
	IsAdult      bool     `json:"is_adult"`
}

// TorrentSearchResult is one candidate torrent from a provider.
type TorrentSearchResult struct {
	Provider      string `json:"provider"`
	Name          string `json:"name"`
	Magnet        string `json:"magnet"`
	Link          string `json:"link"`
	InfoHash      string `json:"info_hash"`
	Date          string `json:"date"`
	Size          int64  `json:"size"`
	Seeders       int    `json:"seeders"`
	Resolution    string `json:"resolution"`
	ReleaseGroup  string `json:"release_group"`
	EpisodeNumber int    `json:"episode_number"`
	IsBatch       bool   `json:"is_batch"`
	IsBestRelease bool   `json:"is_best_release"`
	Confirmed     bool   `json:"confirmed"`
}

// ---- Extensions ----

type CreateExtensionRepoRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// ExtensionDTO is the wire shape for an installed extension.
type ExtensionDTO struct {
	ID         int64   `json:"id"`
	UUID       string  `json:"uuid"`
	RepoID     *int64  `json:"repo_id"`
	ExtID      string  `json:"ext_id"`
	Name       string  `json:"name"`
	Version    *string `json:"version"`
	Lang       string  `json:"lang"`
	Enabled    bool    `json:"enabled"`
	Nsfw       bool    `json:"nsfw"`
	Icon       *string `json:"icon"`
	SourceURL  *string `json:"source_url"`
	AddedAt    int64   `json:"added_at"`
	ModifiedAt int64   `json:"modified_at"`
}

// ExtensionRepoDTO is the wire shape for an extension repo.
type ExtensionRepoDTO struct {
	ID           int64  `json:"id"`
	UUID         string `json:"uuid"`
	Name         string `json:"name"`
	URL          string `json:"url"`
	Enabled      bool   `json:"enabled"`
	LastSyncedAt *int64 `json:"last_synced_at"`
	AddedAt      int64  `json:"added_at"`
}

// toExtensionDTO maps a store row to the frozen wire shape.
func toExtensionDTO(e store.Extension) ExtensionDTO {
	return ExtensionDTO{
		ID:         e.ID,
		UUID:       e.Uuid,
		RepoID:     e.RepoID,
		ExtID:      e.ExtID,
		Name:       e.Name,
		Version:    e.Version,
		Lang:       e.Lang,
		Enabled:    e.Enabled != 0,
		Nsfw:       e.Nsfw != 0,
		Icon:       e.Icon,
		SourceURL:  e.SourceUrl,
		AddedAt:    e.AddedAt,
		ModifiedAt: e.ModifiedAt,
	}
}

// toExtensionRepoDTO maps a store row to the frozen wire shape.
func toExtensionRepoDTO(r store.ExtensionRepo) ExtensionRepoDTO {
	return ExtensionRepoDTO{
		ID:           r.ID,
		UUID:         r.Uuid,
		Name:         r.Name,
		URL:          r.Url,
		Enabled:      r.Enabled != 0,
		LastSyncedAt: r.LastSyncedAt,
		AddedAt:      r.AddedAt,
	}
}

// ---- Logs ----

type LogsResponse struct {
	Lines []string `json:"lines"`
}

// ---- Discovery (home) ----

// DiscoveryItem is one AniList media card on the discovery home. Image fields are
// "" when not on the CSP allowlist (frontend shows a placeholder).
type DiscoveryItem struct {
	AnilistID    int    `json:"anilist_id"`
	RomajiTitle  string `json:"romaji_title"`
	EnglishTitle string `json:"english_title"`
	Format       string `json:"format"`
	Status       string `json:"status"`
	EpisodeCount *int   `json:"episode_count"`
	CoverImage   string `json:"cover_image"`
	BannerImage  string `json:"banner_image"`
	CoverColor   string `json:"cover_color"`
	Season       string `json:"season"`
	SeasonYear   *int   `json:"season_year"`
	IsAdult      bool   `json:"is_adult"`
	// ClearLogoURL is a transparent series-logo URL (ani.zip "Clearlogo"), or ""
	// when absent/unavailable. Only the hero feed's top items are enriched; other
	// cards carry "". The frontend hero shows the logo instead of the text title
	// when present.
	ClearLogoURL string `json:"clear_logo_url"`
	// WideImages is an ordered list of wide hero artwork URLs (ani.zip Fanart then
	// Banner, CSP-safe TVDB hosts), best/sharpest first. Empty when none; only the
	// hero feed's top items are enriched. The hero rotates through these per loop.
	WideImages []string `json:"wide_images"`
}

// DiscoveryRow is one home carousel: a feed key + title + its items. An empty
// items slice tells the frontend to hide the row.
type DiscoveryRow struct {
	Key   string          `json:"key"`
	Title string          `json:"title"`
	Items []DiscoveryItem `json:"items"`
}

// DiscoveryResponse is the full discovery payload (all rows in one call). The
// hero is rows.find(key=='trending').items[0..n] on the frontend.
type DiscoveryResponse struct {
	Rows []DiscoveryRow `json:"rows"`
}

// ---- AniList detail (series-detail page) ----

// AnilistDetail is the frozen wire shape served by GET /api/anilist/{id}/detail.
// It merges AniList Media metadata with ani.zip per-episode metadata into one
// payload the series page (tracked and untracked alike) renders from. Field
// names are part of the frontend contract and must not drift.
type AnilistDetail struct {
	AnilistID       int                `json:"anilist_id"`
	TitleEnglish    string             `json:"title_english"`
	TitleRomaji     string             `json:"title_romaji"`
	CoverImage      string             `json:"cover_image"`
	CoverColor      string             `json:"cover_color"`
	BannerImage     string             `json:"banner_image"`
	Format          string             `json:"format"`
	AiringStatus    string             `json:"airing_status"`
	Description     string             `json:"description"`
	Genres          []string           `json:"genres"`
	AverageScore    int                `json:"average_score"`
	Studio          string             `json:"studio"`
	SourceMaterial  string             `json:"source_material"`
	Season          string             `json:"season"`
	SeasonYear      int                `json:"season_year"`
	DurationMin     int                `json:"duration_min"`
	EpisodeCount    int                `json:"episode_count"`
	NextAiring      *NextAiring        `json:"next_airing"`
	Trailer         *DetailTrailer     `json:"trailer"`
	Episodes        []DetailEpisode    `json:"episodes"`
	Relations       []RelatedMediaCard `json:"relations"`
	Recommendations []RelatedMediaCard `json:"recommendations"`
}

// NextAiring is the next-airing episode number + unix airing time, or null.
type NextAiring struct {
	Episode  int   `json:"episode"`
	AiringAt int64 `json:"airing_at"`
}

// DetailTrailer is an external trailer reference (opened in a new tab; never
// embedded). site is "youtube"/"dailymotion"; video_id is that platform's id.
type DetailTrailer struct {
	Site      string `json:"site"`
	VideoID   string `json:"video_id"`
	Thumbnail string `json:"thumbnail"`
}

// DetailEpisode is one merged episode card: ani.zip-primary metadata with an
// AniList streamingEpisodes fallback. Empty fields tell the frontend to render
// a tinted placeholder.
type DetailEpisode struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	Thumbnail  string `json:"thumbnail"`
	AirDate    string `json:"air_date"`
	Overview   string `json:"overview"`
	RuntimeMin int    `json:"runtime_min"`
}

// RelatedMediaCard is a relation or recommendation entry, carrying enough to
// render a discovery card and navigate to its preview. relation_type is empty
// for recommendations.
type RelatedMediaCard struct {
	AnilistID    int    `json:"anilist_id"`
	RelationType string `json:"relation_type,omitempty"`
	TitleEnglish string `json:"title_english"`
	TitleRomaji  string `json:"title_romaji"`
	CoverImage   string `json:"cover_image"`
	CoverColor   string `json:"cover_color"`
	Format       string `json:"format"`
	Status       string `json:"status"`
}

// ---- Tracked (home "Currently downloading" + Downloads grid) ----

// TrackedResponse buckets tracked series by status for the home + Downloads grid.
type TrackedResponse struct {
	InProgress []SeriesProgress `json:"in_progress"`
	Completed  []SeriesProgress `json:"completed"`
	Paused     []SeriesProgress `json:"paused"`
	Dropped    []SeriesProgress `json:"dropped"`
}

// ---- Track / status override ----

// TrackRequest is the "Download & track" body.
type TrackRequest struct {
	AnilistID int64 `json:"anilist_id"`
}

// TrackResponse is returned by POST /api/track (201 create, 200 idempotent).
type TrackResponse struct {
	Series   SeriesProgress `json:"series"`
	SeriesID int64          `json:"series_id"`
	FeedID   int64          `json:"feed_id"`
}

// SeriesStatusResponse wraps a single series for the set-status endpoint.
type SeriesStatusResponse struct {
	Series SeriesProgress `json:"series"`
}

// SetStatusRequest is the POST /api/series/{id}/status body: the new watch status
// (watching | on_hold | dropped). 'completed' is derived and not settable here.
type SetStatusRequest struct {
	Status string `json:"status"`
}

// EpisodeRetryResponse wraps the requeued episode for POST /api/episodes/{id}/retry.
type EpisodeRetryResponse struct {
	Episode EpisodeDetail `json:"episode"`
}

// ---- Activity ----

// ActivitySeries is one subscribed series plus its full episode record for the
// Activity page. It carries every SeriesProgress field (incl status, poster,
// derived_status) and the series' episodes newest-first.
type ActivitySeries struct {
	SeriesProgress
	Episodes []EpisodeDetail `json:"episodes"`
}

// ActivityResponse is the GET /api/activity payload: all subscribed series with
// their episodes, ordered active-series-first then by most-recent activity.
type ActivityResponse struct {
	Series []ActivitySeries `json:"series"`
}

// ---- Available episodes (on-demand source check) ----

// AvailableEpisode is one source-available, not-yet-downloaded episode.
type AvailableEpisode struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	SourceURL  string `json:"source_url"`
	Size       *int64 `json:"size"`
	Resolution string `json:"resolution"`
}

// AvailableResponse is the GET /api/anilist/{id}/available payload. Warnings
// carries one human-readable message per provider that failed, so a user with a
// dead source sees the cause instead of a silently empty episode list.
type AvailableResponse struct {
	Episodes []AvailableEpisode `json:"episodes"`
	Warnings []string           `json:"warnings,omitempty"`
}

// DownloadAvailableRequest is the POST /api/anilist/{id}/available/download body:
// a source-found episode the user chose to download (subscribed or not).
type DownloadAvailableRequest struct {
	SourceURL  string `json:"source_url"`
	Number     int    `json:"number"`
	Resolution string `json:"resolution"`
}

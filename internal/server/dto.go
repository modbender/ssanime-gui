package server

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
	ID           int64           `json:"id"`
	UUID         string          `json:"uuid"`
	SeriesID     int64           `json:"series_id"`
	Title        *string         `json:"title"`
	EpisodeNo    *int64          `json:"episode_no"`
	Status       string          `json:"status"`
	Resolution   *int64          `json:"resolution"`
	ReleaseGroup *string         `json:"release_group"`
	Subtype      *string         `json:"subtype"`
	Uncensored   bool            `json:"uncensored"`
	Bluray       bool            `json:"bluray"`
	SourceSize   *int64          `json:"source_size"`
	ProfileID    *int64          `json:"profile_id"`
	ErrorMessage *string         `json:"error_message"`
	RetryCount   int64           `json:"retry_count"`
	PublishedAt  *int64          `json:"published_at"`
	DownloadedAt *int64          `json:"downloaded_at"`
	EncodedAt    *int64          `json:"encoded_at"`
	Outputs      []OutputSummary `json:"outputs"`
	AddedAt      int64           `json:"added_at"`
	ModifiedAt   int64           `json:"modified_at"`
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

// ---- Feed DTOs ----

type CreateFeedRequest struct {
	SeriesID        int64   `json:"series_id"`
	Type            string  `json:"type"`
	Site            *string `json:"site"`
	URL             string  `json:"url"`
	Quality         *int64  `json:"quality"`
	Subtype         *string `json:"subtype"`
	Deinterlace     bool    `json:"deinterlace"`
	Uncensored      bool    `json:"uncensored"`
	Bluray          bool    `json:"bluray"`
	TitleRegex      *string `json:"title_regex"`
	ExtraTags       *string `json:"extra_tags"`
	IntervalSeconds int64   `json:"interval_seconds"`
	OffsetSeconds   int64   `json:"offset_seconds"`
	Enabled         bool    `json:"enabled"`
}

type PatchFeedRequest struct {
	Type            *string `json:"type"`
	Site            *string `json:"site"`
	URL             *string `json:"url"`
	Quality         *int64  `json:"quality"`
	Subtype         *string `json:"subtype"`
	Deinterlace     *bool   `json:"deinterlace"`
	Uncensored      *bool   `json:"uncensored"`
	Bluray          *bool   `json:"bluray"`
	TitleRegex      *string `json:"title_regex"`
	ExtraTags       *string `json:"extra_tags"`
	IntervalSeconds *int64  `json:"interval_seconds"`
	OffsetSeconds   *int64  `json:"offset_seconds"`
	Enabled         *bool   `json:"enabled"`
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

// ---- Logs ----

type LogsResponse struct {
	Lines []string `json:"lines"`
}

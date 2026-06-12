// Package source is the sourcing layer: it finds anime torrent releases for a
// series/episode across pluggable providers, parses release names with habari,
// and autoselects the best original release.
//
// The Provider interface is a native-Go reimplementation of the hibike
// AnimeProvider shape. Providers are supplied at runtime by user-installed JS
// extensions running in the goja runtime, registered into source.Registry as
// they install and enable.
package source

import "context"

// ProviderType classifies a provider. Main providers can be used as defaults;
// special providers (e.g. adult-only) are excluded from GetLatest defaults.
type ProviderType string

const (
	ProviderTypeMain    ProviderType = "main"
	ProviderTypeSpecial ProviderType = "special"
)

// SmartSearchFilter is a capability flag a provider advertises in its settings.
type SmartSearchFilter string

const (
	FilterBatch         SmartSearchFilter = "batch"
	FilterEpisodeNumber SmartSearchFilter = "episodeNumber"
	FilterResolution    SmartSearchFilter = "resolution"
	FilterQuery         SmartSearchFilter = "query"
	FilterBestReleases  SmartSearchFilter = "bestReleases"
)

// Settings describes what a provider can do; mirrors hibike AnimeProviderSettings.
type Settings struct {
	CanSmartSearch     bool                `json:"canSmartSearch"`
	SmartSearchFilters []SmartSearchFilter `json:"smartSearchFilters"`
	SupportsAdult      bool                `json:"supportsAdult"`
	Type               ProviderType        `json:"type"`
}

// Media is the metadata input for SmartSearch: AniList/AniDB identity plus
// titles and episode count used to build queries and confirm matches. Mirrors
// the hibike Media struct (the field set goja extensions already expect).
type Media struct {
	// ID is the AniList ID of the media.
	ID int `json:"id"`
	// IDMal is the MyAnimeList ID.
	IDMal *int `json:"idMal,omitempty"`
	// AniDB anime/episode IDs, when known (enable Confirmed matches).
	AnidbAID int `json:"anidbAID,omitempty"`
	AnidbEID int `json:"anidbEID,omitempty"`
	// Status: FINISHED|RELEASING|NOT_YET_RELEASED|CANCELLED|HIATUS.
	Status string `json:"status,omitempty"`
	// Format: TV|TV_SHORT|MOVIE|SPECIAL|OVA|ONA|MUSIC.
	Format string `json:"format,omitempty"`
	// EnglishTitle is nil when unknown.
	EnglishTitle *string `json:"englishTitle,omitempty"`
	// RomajiTitle is the primary romaji title.
	RomajiTitle string `json:"romajiTitle,omitempty"`
	// EpisodeCount is -1 when unknown / not applicable.
	EpisodeCount int `json:"episodeCount,omitempty"`
	// Synonyms are alternative titles used for fuzzy matching.
	Synonyms []string `json:"synonyms,omitempty"`
	// IsAdult flags NSFW media.
	IsAdult bool `json:"isAdult,omitempty"`
}

// Titles returns every non-empty title (english, romaji, synonyms) for matching.
func (m Media) Titles() []string {
	var out []string
	if m.EnglishTitle != nil && *m.EnglishTitle != "" {
		out = append(out, *m.EnglishTitle)
	}
	if m.RomajiTitle != "" {
		out = append(out, m.RomajiTitle)
	}
	for _, s := range m.Synonyms {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// SearchOptions are the inputs for a plain (unfiltered) Search.
type SearchOptions struct {
	Media Media  `json:"media"`
	Query string `json:"query"`
}

// SmartSearchOptions are the inputs for a filtered, metadata-aware search.
type SmartSearchOptions struct {
	Media Media `json:"media"`
	// Query is empty if the provider doesn't support custom queries.
	Query string `json:"query"`
	// Batch requests batch torrents.
	Batch bool `json:"batch"`
	// EpisodeNumber is 0 if the provider doesn't filter by episode.
	EpisodeNumber int `json:"episodeNumber"`
	// Resolution is "" if the provider doesn't filter by resolution. e.g. "1080".
	Resolution string `json:"resolution"`
	// BestReleases requests only releases the provider can confirm are best.
	BestReleases bool `json:"bestReleases"`
}

// AnimeTorrent is one normalized release result. Field set mirrors hibike
// AnimeTorrent so the same shape feeds autoselect and (later) JS extensions.
type AnimeTorrent struct {
	// Provider is the registry id of the source that produced this result.
	Provider string `json:"provider,omitempty"`
	// Name is the raw release title (parsed by habari into the fields below).
	Name string `json:"name"`
	// Magnet is a direct magnet link, or "" if it must be resolved later.
	Magnet string `json:"magnet,omitempty"`
	// Link is the torrent page or .torrent download URL.
	Link string `json:"link,omitempty"`
	// InfoHash is the BitTorrent info hash, or "" if not yet known.
	InfoHash string `json:"infoHash,omitempty"`
	// Date is the publish time in RFC3339, or "" if unknown.
	Date string `json:"date,omitempty"`
	// Size is the torrent size in bytes, or 0 if unknown.
	Size int64 `json:"size,omitempty"`
	// Seeders is the seeder count, or -1 if unknown.
	Seeders int `json:"seeders"`
	// Leechers is the leecher count, or -1 if unknown.
	Leechers int `json:"leechers,omitempty"`
	// Resolution e.g. "1080p", parsed from the name when not provided.
	Resolution string `json:"resolution,omitempty"`
	// ReleaseGroup e.g. "SubsPlease", parsed from the name when not provided.
	ReleaseGroup string `json:"releaseGroup,omitempty"`
	// EpisodeNumber is the parsed episode, or -1 if unknown / a batch.
	EpisodeNumber int `json:"episodeNumber"`
	// IsBatch is true when the release packs multiple episodes.
	IsBatch bool `json:"isBatch,omitempty"`
	// IsBestRelease is true when the provider marks it the canonical best release.
	IsBestRelease bool `json:"isBestRelease,omitempty"`
	// Confirmed is true when the result is confirmed to match the requested media
	// (e.g. via AniDB id), not just a fuzzy title match.
	Confirmed bool `json:"confirmed,omitempty"`
}

// Provider is the native-Go reimplementation of the hibike AnimeProvider
// interface. Every source (native or, later, a goja-hosted JS extension)
// implements it.
type Provider interface {
	// ID is the stable registry key for this provider (e.g. "nyaa").
	ID() string
	// Search runs an unfiltered query.
	Search(ctx context.Context, opts SearchOptions) ([]*AnimeTorrent, error)
	// SmartSearch runs a metadata-aware, filtered query.
	SmartSearch(ctx context.Context, opts SmartSearchOptions) ([]*AnimeTorrent, error)
	// GetLatest returns the newest releases (provider homepage feed).
	GetLatest(ctx context.Context) ([]*AnimeTorrent, error)
	// GetTorrentMagnetLink returns the magnet for a result, resolving it from the
	// torrent page only if it isn't already present.
	GetTorrentMagnetLink(ctx context.Context, t *AnimeTorrent) (string, error)
	// GetTorrentInfoHash returns the info hash for a result, resolving it only if
	// it isn't already present.
	GetTorrentInfoHash(ctx context.Context, t *AnimeTorrent) (string, error)
	// GetSettings reports the provider's capabilities.
	GetSettings() Settings
}

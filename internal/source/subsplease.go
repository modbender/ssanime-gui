package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ProviderSubsPlease is the registry id for the SubsPlease provider.
const ProviderSubsPlease = "subsplease"

// subspleaseAPI is the structured JSON endpoint. f=search&s=<query> returns a
// map keyed by "<show> - <episode>", each with pre-parsed per-resolution magnets
// — no fuzzy parsing needed. f=latest returns the recent global feed. Reachable
// directly (not DNS-blocked), so it uses the direct client.
const subspleaseAPI = "https://subsplease.org/api/"

// subspleaseGroup is the constant release group for everything SubsPlease ships.
const subspleaseGroup = "SubsPlease"

// SubsPlease is a structured provider: it returns pre-parsed magnets per episode
// per resolution from the SubsPlease show API, so results are reliable without a
// fuzzy match step.
type SubsPlease struct {
	client *http.Client
}

// NewSubsPlease builds the provider over the given direct HTTP client.
func NewSubsPlease(client *http.Client) *SubsPlease {
	return &SubsPlease{client: client}
}

// ID implements Provider.
func (s *SubsPlease) ID() string { return ProviderSubsPlease }

// GetSettings implements Provider.
func (s *SubsPlease) GetSettings() Settings {
	return Settings{
		CanSmartSearch: true,
		SmartSearchFilters: []SmartSearchFilter{
			FilterQuery, FilterResolution, FilterEpisodeNumber, FilterBatch,
		},
		SupportsAdult: false,
		Type:          ProviderTypeMain,
	}
}

// spEntry is one show/episode group from the SubsPlease API.
type spEntry struct {
	Show        string `json:"show"`
	Episode     string `json:"episode"`
	ReleaseDate string `json:"release_date"`
	Downloads   []struct {
		Res    string `json:"res"`
		Magnet string `json:"magnet"`
	} `json:"downloads"`
}

// Search implements Provider: a raw query against the SubsPlease search API.
func (s *SubsPlease) Search(ctx context.Context, opts SearchOptions) ([]*AnimeTorrent, error) {
	return s.search(ctx, opts.Query)
}

// SmartSearch implements Provider: queries by the best media title, then filters
// the structured results by the requested episode/resolution.
func (s *SubsPlease) SmartSearch(ctx context.Context, opts SmartSearchOptions) ([]*AnimeTorrent, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		query = preferredTitle(opts.Media)
	}
	results, err := s.search(ctx, query)
	if err != nil {
		return nil, err
	}
	return filterSmart(results, opts), nil
}

// GetLatest implements Provider: the recent global SubsPlease feed.
func (s *SubsPlease) GetLatest(ctx context.Context) ([]*AnimeTorrent, error) {
	return s.fetch(ctx, subspleaseAPI+"?f=latest&tz=UTC")
}

// GetTorrentMagnetLink implements Provider. SubsPlease always supplies a magnet.
func (s *SubsPlease) GetTorrentMagnetLink(_ context.Context, t *AnimeTorrent) (string, error) {
	if t.Magnet != "" {
		return t.Magnet, nil
	}
	return "", fmt.Errorf("subsplease: no magnet for %q", t.Name)
}

// GetTorrentInfoHash implements Provider.
func (s *SubsPlease) GetTorrentInfoHash(_ context.Context, t *AnimeTorrent) (string, error) {
	if t.InfoHash != "" {
		return t.InfoHash, nil
	}
	if mm := infoHashRe.FindStringSubmatch(t.Magnet); mm != nil {
		return strings.ToLower(mm[1]), nil
	}
	return "", fmt.Errorf("subsplease: no info hash for %q", t.Name)
}

// search runs a search query and normalizes the results.
func (s *SubsPlease) search(ctx context.Context, query string) ([]*AnimeTorrent, error) {
	u := subspleaseAPI + "?f=search&tz=UTC&s=" + url.QueryEscape(strings.TrimSpace(query))
	return s.fetch(ctx, u)
}

// fetch GETs a SubsPlease API URL and decodes its keyed-map response into
// normalized torrents. The API returns an empty JSON array (not an object) when
// there are no results, which decodes to no entries.
func (s *SubsPlease) fetch(ctx context.Context, apiURL string) ([]*AnimeTorrent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ssanime-gui/1.0")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("subsplease: fetch %s: %w", apiURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subsplease: %s returned %s", apiURL, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFeedBytes))
	if err != nil {
		return nil, err
	}

	// No results: the API returns `[]` instead of an object.
	if strings.TrimSpace(string(body)) == "[]" {
		return nil, nil
	}
	var raw map[string]spEntry
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("subsplease: decode (%d bytes): %w", len(body), err)
	}

	var out []*AnimeTorrent
	for key, e := range raw {
		batch := strings.Contains(e.Episode, "-")
		for _, d := range e.Downloads {
			name := fmt.Sprintf("[SubsPlease] %s - %s (%sp)", e.Show, e.Episode, d.Res)
			t := &AnimeTorrent{
				Provider:     ProviderSubsPlease,
				Name:         name,
				Magnet:       d.Magnet,
				Date:         spDate(e.ReleaseDate),
				Seeders:      -1,
				Leechers:     -1,
				Resolution:   d.Res + "p",
				ReleaseGroup: subspleaseGroup,
				IsBatch:      batch,
			}
			if mm := infoHashRe.FindStringSubmatch(d.Magnet); mm != nil {
				t.InfoHash = strings.ToLower(mm[1])
			}
			if batch {
				t.EpisodeNumber = -1
			} else {
				t.EpisodeNumber = atoiSafe(strings.TrimLeft(e.Episode, "0"))
			}
			_ = key
			out = append(out, t)
		}
	}
	return out, nil
}

// spDate normalizes the SubsPlease RFC1123Z release_date to RFC3339, leaving the
// original string if it can't be parsed.
func spDate(s string) string {
	const rfc1123z = "Mon, 02 Jan 2006 15:04:05 -0700"
	if t, err := parseTime(rfc1123z, s); err == nil {
		return t.UTC().Format("2006-01-02T15:04:05Z07:00")
	}
	return s
}

package source

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/mmcdole/gofeed"
	ext "github.com/mmcdole/gofeed/extensions"
)

// ProviderNyaa is the registry id for the nyaa.si provider.
const ProviderNyaa = "nyaa"

// nyaaRSSBase is the seeders-sorted anime-English RSS endpoint. Category 1_2 is
// "Anime - English-translated". Fetched through the DoH client because the ISP
// DNS-poisons nyaa.si (verified); TLS SNI keeps the real host so certs validate.
const nyaaRSSBase = "https://nyaa.si/?page=rss&c=1_2&f=0&s=seeders&o=desc&q="

// nyaaLatestURL is the unfiltered seeders-sorted feed (no query) for GetLatest.
const nyaaLatestURL = "https://nyaa.si/?page=rss&c=1_2&f=0&s=id&o=desc"

// queryClean strips characters nyaa's search treats as noise.
var queryClean = regexp.MustCompile(`[^\w\s-]`)

// Nyaa is a provider over nyaa.si RSS. It understands no series/episode identity
// on its own; habari parsing + the autoselect matcher supply that.
type Nyaa struct {
	client *http.Client
	parser *gofeed.Parser
}

// NewNyaa builds the nyaa provider over the given (DoH-backed) client.
func NewNyaa(client *http.Client) *Nyaa {
	fp := gofeed.NewParser()
	fp.UserAgent = "ssanime-gui/1.0"
	fp.Client = client
	return &Nyaa{client: client, parser: fp}
}

// ID implements Provider.
func (n *Nyaa) ID() string { return ProviderNyaa }

// GetSettings implements Provider.
func (n *Nyaa) GetSettings() Settings {
	return Settings{
		CanSmartSearch: true,
		SmartSearchFilters: []SmartSearchFilter{
			FilterQuery, FilterResolution, FilterEpisodeNumber, FilterBatch,
		},
		SupportsAdult: false,
		Type:          ProviderTypeMain,
	}
}

// Search implements Provider: a raw query against nyaa RSS.
func (n *Nyaa) Search(ctx context.Context, opts SearchOptions) ([]*AnimeTorrent, error) {
	return n.fetch(ctx, nyaaRSSBase+url.QueryEscape(cleanQuery(opts.Query)))
}

// SmartSearch implements Provider: builds a query from the media titles + the
// requested episode/resolution, then filters the parsed results to match.
func (n *Nyaa) SmartSearch(ctx context.Context, opts SmartSearchOptions) ([]*AnimeTorrent, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		query = preferredTitle(opts.Media)
	}
	results, err := n.fetch(ctx, nyaaRSSBase+url.QueryEscape(cleanQuery(query)))
	if err != nil {
		return nil, err
	}
	return filterSmart(results, opts), nil
}

// GetLatest implements Provider: the newest releases, no query.
func (n *Nyaa) GetLatest(ctx context.Context) ([]*AnimeTorrent, error) {
	return n.fetch(ctx, nyaaLatestURL)
}

// GetTorrentMagnetLink implements Provider: nyaa RSS already carries enough to
// build a magnet from the info hash, so no page scrape is needed.
func (n *Nyaa) GetTorrentMagnetLink(_ context.Context, t *AnimeTorrent) (string, error) {
	if m := ensureMagnet(t); m != "" {
		return m, nil
	}
	return "", fmt.Errorf("nyaa: no magnet or info hash for %q", t.Name)
}

// GetTorrentInfoHash implements Provider.
func (n *Nyaa) GetTorrentInfoHash(_ context.Context, t *AnimeTorrent) (string, error) {
	if t.InfoHash != "" {
		return t.InfoHash, nil
	}
	if mm := infoHashRe.FindStringSubmatch(t.Magnet); mm != nil {
		return strings.ToLower(mm[1]), nil
	}
	return "", fmt.Errorf("nyaa: no info hash for %q", t.Name)
}

// fetch parses a nyaa RSS URL into normalized, habari-enriched torrents.
func (n *Nyaa) fetch(ctx context.Context, feedURL string) ([]*AnimeTorrent, error) {
	feed, err := n.parser.ParseURLWithContext(feedURL, ctx)
	if err != nil {
		return nil, fmt.Errorf("nyaa: fetch %s: %w", feedURL, err)
	}
	out := make([]*AnimeTorrent, 0, len(feed.Items))
	for _, item := range feed.Items {
		t := &AnimeTorrent{
			Provider: ProviderNyaa,
			Name:     item.Title,
			Link:     item.Link,
			Seeders:  -1,
			Leechers: -1,
		}
		if item.PublishedParsed != nil {
			t.Date = item.PublishedParsed.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
		if nyaa, ok := item.Extensions["nyaa"]; ok {
			if v := extVal(nyaa, "seeders"); v != "" {
				t.Seeders = atoiSafe(v)
			}
			if v := extVal(nyaa, "leechers"); v != "" {
				t.Leechers = atoiSafe(v)
			}
			if v := extVal(nyaa, "infoHash"); v != "" {
				t.InfoHash = strings.ToLower(v)
			}
			if v := extVal(nyaa, "size"); v != "" {
				t.Size = parseSize(v)
			}
		}
		enrich(t)
		t.Magnet = ensureMagnet(t)
		out = append(out, t)
	}
	return out, nil
}

// cleanQuery normalizes a search query the way nyaa's search likes it.
func cleanQuery(q string) string {
	return strings.TrimSpace(queryClean.ReplaceAllString(q, " "))
}

// extVal reads the first value of a gofeed extension key.
func extVal(m map[string][]ext.Extension, key string) string {
	if v, ok := m[key]; ok && len(v) > 0 {
		return v[0].Value
	}
	return ""
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// sizeRe parses nyaa's human size strings ("1.4 GiB", "350.2 MiB").
var sizeRe = regexp.MustCompile(`(?i)([\d.]+)\s*(B|KiB|MiB|GiB|TiB|KB|MB|GB|TB)`)

func parseSize(s string) int64 {
	m := sizeRe.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	val, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	mult := map[string]float64{
		"B": 1, "KIB": 1 << 10, "MIB": 1 << 20, "GIB": 1 << 30, "TIB": 1 << 40,
		"KB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12,
	}[strings.ToUpper(m[2])]
	return int64(val * mult)
}

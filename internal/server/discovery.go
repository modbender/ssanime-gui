package server

import (
	"net/http"

	"github.com/modbender/ssanime-gui/internal/anilist"
	"github.com/modbender/ssanime-gui/internal/discovery"
)

// handleDiscovery returns every discovery row in one payload, read from the
// server-side cache (zero AniList calls per page-load). A cold cache yields rows
// with empty items — always 200, never an error — so the frontend skeletons or
// hides those rows. Rows are emitted in the static feed order even before the
// cache warms, so the layout is stable from first paint.
func (h *Handler) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	rows := make([]DiscoveryRow, 0, len(discovery.Feeds()))
	var snap map[discovery.FeedKey][]anilist.Media
	if h.discovery != nil {
		snap = h.discovery.Snapshot()
	}
	for _, f := range discovery.Feeds() {
		items := make([]DiscoveryItem, 0)
		for _, m := range snap[f.Key] {
			items = append(items, mediaToDiscoveryItem(m))
		}
		rows = append(rows, DiscoveryRow{
			Key:   string(f.Key),
			Title: f.Title,
			Items: items,
		})
	}
	WriteJSON(w, http.StatusOK, DiscoveryResponse{Rows: rows})
}

// mediaToDiscoveryItem maps an anilist.Media to the frozen DiscoveryItem wire
// shape. Image URLs are already CSP-pinned by the anilist mapper (non-allowlisted
// hosts arrive as ""); episode_count/season_year are nullable so 0 (unknown) maps
// to JSON null instead of a misleading zero.
func mediaToDiscoveryItem(m anilist.Media) DiscoveryItem {
	item := DiscoveryItem{
		AnilistID:    m.ID,
		RomajiTitle:  m.RomajiTitle,
		EnglishTitle: m.EnglishTitle,
		Format:       m.Format,
		Status:       m.Status,
		CoverImage:   m.CoverImage,
		BannerImage:  m.BannerImage,
		CoverColor:   m.CoverColor,
		Season:       m.Season,
		IsAdult:      m.IsAdult,
		ClearLogoURL: m.ClearLogoURL,
		WideImages:   m.WideImages,
	}
	// Serialize as a JSON array, never null, when the item carries no wide art.
	if item.WideImages == nil {
		item.WideImages = []string{}
	}
	if m.EpisodeCount > 0 {
		ec := m.EpisodeCount
		item.EpisodeCount = &ec
	}
	if m.SeasonYear > 0 {
		sy := m.SeasonYear
		item.SeasonYear = &sy
	}
	return item
}

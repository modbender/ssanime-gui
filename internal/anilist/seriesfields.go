package anilist

import "encoding/json"

// SeriesFields is the AniList-derived subset of a series row, in the exact shape
// the store expects (nullable columns as *T, synonyms pre-marshalled to a JSON
// string). It is the single place the Media -> series-column mapping lives; both
// the create-series path and the background metadata refresh build their sqlc
// params from one of these, so the field list never drifts between them.
//
// Title is the chosen display title (English when present, else Romaji); the
// refresh path deliberately ignores it (title is the unique display key), while
// create uses it to seed the new row.
type SeriesFields struct {
	Title        string
	AnilistID    *int64
	MalID        *int64
	RomajiTitle  *string
	EnglishTitle *string
	Format       *string
	Status       *string
	AiringStatus *string
	EpisodeCount *int64
	Synonyms     *string
	CoverImage   *string
	BannerImage  *string
	CoverColor   *string
	Season       *string
	SeasonYear   *int64
}

// MediaToSeriesFields maps a fetched Media to the series-column values. Empty
// strings/zero counts map to nil so a sparse upstream response never writes a
// blank over a previously-populated column (the refresh query's COALESCE handles
// the preserve-on-empty case for the fields it guards).
func MediaToSeriesFields(m Media) SeriesFields {
	f := SeriesFields{
		Title:        m.RomajiTitle,
		RomajiTitle:  strPtr(m.RomajiTitle),
		Format:       strPtr(m.Format),
		Status:       strPtr(m.Status),
		AiringStatus: strPtr(m.Status),
		CoverImage:   strPtr(m.CoverImage),
		BannerImage:  strPtr(m.BannerImage),
		CoverColor:   strPtr(m.CoverColor),
		Season:       strPtr(m.Season),
	}
	if m.ID != 0 {
		id := int64(m.ID)
		f.AnilistID = &id
	}
	if m.IDMal != nil {
		mal := int64(*m.IDMal)
		f.MalID = &mal
	}
	if m.EnglishTitle != "" {
		f.EnglishTitle = strPtr(m.EnglishTitle)
		f.Title = m.EnglishTitle
	}
	if m.EpisodeCount > 0 {
		ec := int64(m.EpisodeCount)
		f.EpisodeCount = &ec
	}
	if m.SeasonYear != 0 {
		sy := int64(m.SeasonYear)
		f.SeasonYear = &sy
	}
	if len(m.Synonyms) > 0 {
		if raw, err := json.Marshal(m.Synonyms); err == nil {
			s := string(raw)
			f.Synonyms = &s
		}
	}
	return f
}

// strPtr returns nil for the empty string, else a pointer to s.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

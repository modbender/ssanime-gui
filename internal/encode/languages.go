package encode

// Language is one entry in the curated common-language list: the normalized ISO
// 639-1 code stored in profiles and a human display name for the UI.
type Language struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// CommonLanguages is the curated list of selectable track languages, shared as
// the single source of truth for REST validation; the frontend mirrors it.
// Codes match the normalized output of normalizeLang. Adding a language is one
// entry here (plus its aliases in langAliases for tag normalization).
var CommonLanguages = []Language{
	{Code: "en", Name: "English"},
	{Code: "ja", Name: "Japanese"},
	{Code: "es", Name: "Spanish"},
	{Code: "pt", Name: "Portuguese"},
	{Code: "fr", Name: "French"},
	{Code: "de", Name: "German"},
	{Code: "it", Name: "Italian"},
	{Code: "ru", Name: "Russian"},
	{Code: "zh", Name: "Chinese"},
	{Code: "ko", Name: "Korean"},
	{Code: "ar", Name: "Arabic"},
}

// knownLanguageCodes indexes CommonLanguages by code for O(1) validation.
var knownLanguageCodes = func() map[string]bool {
	m := make(map[string]bool, len(CommonLanguages))
	for _, l := range CommonLanguages {
		m[l.Code] = true
	}
	return m
}()

// IsKnownLanguage reports whether a code is in the curated common-language list.
func IsKnownLanguage(code string) bool { return knownLanguageCodes[code] }

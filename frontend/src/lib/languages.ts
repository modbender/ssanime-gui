// Curated common-language list for per-track audio/subtitle selection.
// Mirrors the Go source of truth `encode.CommonLanguages` (internal/encode/languages.go);
// codes match normalizeLang's output and are validated server-side (unknown → HTTP 400).
// Adding a language is one entry here plus the Go mirror.

export interface Language {
  code: string
  name: string
}

export const COMMON_LANGUAGES: readonly Language[] = [
  { code: 'en', name: 'English' },
  { code: 'ja', name: 'Japanese' },
  { code: 'es', name: 'Spanish' },
  { code: 'pt', name: 'Portuguese' },
  { code: 'fr', name: 'French' },
  { code: 'de', name: 'German' },
  { code: 'it', name: 'Italian' },
  { code: 'ru', name: 'Russian' },
  { code: 'zh', name: 'Chinese' },
  { code: 'ko', name: 'Korean' },
  { code: 'ar', name: 'Arabic' },
]

const NAME_BY_CODE = new Map(COMMON_LANGUAGES.map((l) => [l.code, l.name]))

export function languageName(code: string): string {
  return NAME_BY_CODE.get(code) ?? code
}

// Typed API client — unwraps the {data, error} envelope from every endpoint.

export interface ApiResponse<T> {
  data: T | null
  error: string | null
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const base = '/api'
  const opts: RequestInit = {
    method,
    headers: { 'Content-Type': 'application/json' },
  }
  if (body !== undefined) {
    opts.body = JSON.stringify(body)
  }
  const res = await fetch(`${base}${path}`, opts)
  if (!res.ok && res.status === 0) {
    throw new Error('Network error')
  }
  const envelope: ApiResponse<T> = await res.json()
  if (envelope.error) {
    throw new Error(envelope.error)
  }
  return envelope.data as T
}

const get = <T>(path: string) => request<T>('GET', path)
const post = <T>(path: string, body?: unknown) => request<T>('POST', path, body)
const patch = <T>(path: string, body?: unknown) => request<T>('PATCH', path, body)
const put = <T>(path: string, body?: unknown) => request<T>('PUT', path, body)
const del = <T>(path: string) => request<T>('DELETE', path)

// ---- Types (mirrors Go DTOs) ----

export interface SeriesProgress {
  id: number
  uuid: string
  title: string
  feed_title: string | null
  season_number: number
  subscribed: boolean
  favorite: boolean
  airing_status: string | null
  derived_status: string
  /** manual override layer: null = automatic, 'paused' | 'dropped' */
  user_status: string | null
  poster_path: string | null
  cover_image_url: string | null
  banner_image_url: string | null
  cover_color: string | null
  anilist_id: number | null
  romaji_title: string | null
  english_title: string | null
  format: string | null
  episode_count: number | null
  episode_total: number
  episode_archived: number
  source_bytes_total: number
  encoded_bytes_total: number
  space_saved_bytes: number
  added_at: number
  modified_at: number
}

export interface OutputSummary {
  id: number
  uuid: string
  resolution: number
  status: string
  encoded_path: string | null
  encoded_size: number | null
  error_message: string | null
  encoded_at: number | null
}

export interface EpisodeDetail {
  id: number
  uuid: string
  series_id: number
  title: string | null
  episode_no: number | null
  status: string
  resolution: number | null
  release_group: string | null
  subtype: string | null
  uncensored: boolean
  bluray: boolean
  source_size: number | null
  profile_id: number | null
  error_message: string | null
  retry_count: number
  published_at: number | null
  downloaded_at: number | null
  encoded_at: number | null
  outputs: OutputSummary[]
  added_at: number
  modified_at: number
}

export interface SeriesDetail {
  id: number
  uuid: string
  title: string
  feed_title: string | null
  alt_titles: string | null
  season_number: number
  subscribed: boolean
  favorite: boolean
  airing_status: string | null
  derived_status: string
  /** manual override layer: null = automatic, 'paused' | 'dropped' */
  user_status: string | null
  poster_path: string | null
  cover_image_url: string | null
  banner_image_url: string | null
  cover_color: string | null
  anilist_id: number | null
  romaji_title: string | null
  english_title: string | null
  format: string | null
  episode_count: number | null
  default_profile_id: number | null
  episodes: EpisodeDetail[]
  added_at: number
  modified_at: number
}

export interface AnilistSearchResult {
  id: number
  idMal: number | null
  romaji_title: string
  english_title: string
  format: string
  status: string
  episode_count: number
  cover_image: string
  banner_image: string
  season: string
  season_year: number
  synonyms: string[]
  is_adult: boolean
}

export interface TorrentSearchResult {
  provider: string
  name: string
  magnet: string
  link: string
  info_hash: string
  date: string
  size: number
  seeders: number
  resolution: string
  release_group: string
  episode_number: number
  is_batch: boolean
  is_best_release: boolean
  confirmed: boolean
}

export interface Feed {
  id: number
  uuid: string
  series_id: number
  type: string
  site: string | null
  url: string
  quality: number | null
  subtype: string | null
  deinterlace: boolean
  uncensored: boolean
  bluray: boolean
  title_regex: string | null
  extra_tags: string | null
  interval_seconds: number
  offset_seconds: number
  enabled: boolean
  last_checked: number | null
}

export interface Profile {
  id: number
  uuid: string
  name: string
  parent_id: number | null
  is_builtin: boolean
  codec: string | null
  crf: number | null
  preset: string | null
  smartblur: boolean | null
  deinterlace: boolean | null
  deblock: string | null
  psy_rd: number | null
  psy_rdoq: number | null
  aq_strength: number | null
  aq_mode: number | null
  scale: number | null
  audio: string | null
  container: string | null
  x265_params: string | null
  output_resolutions: number[] | null
  added_at: number
  modified_at: number
}

export interface ResolvedProfile {
  profile_id: number
  codec: string
  crf: number
  preset: string
  smartblur: boolean
  deinterlace: boolean
  deblock: string
  psy_rd: number
  psy_rdoq: number
  aq_strength: number
  aq_mode: number
  audio: string
  container: string
  x265_params: string
  output_resolutions: number[]
}

export interface Settings {
  download_root: string
  encoded_root: string
  cleanup_policy: string
  processed_dir: string | null
  naming_template: string
  download_backend: number | null
  default_profile_id: number | null
  concurrency_download: number
  concurrency_encode: number
  ffmpeg_path: string | null
  ytdlp_path: string | null
  port: number
  doh_enabled: boolean
}

export interface StatsResponse {
  series_total: number
  episodes_archived: number
  source_bytes_total: number
  encoded_bytes_total: number
  space_saved_bytes: number
}

export interface QueueSnapshot {
  downloading: EpisodeDetail[]
  encoding: EpisodeDetail[]
}

export interface LogsResponse {
  lines: string[]
}

// ---- Discovery + tracking (discovery-first home) ----

export interface DiscoveryItem {
  anilist_id: number
  romaji_title: string
  english_title: string
  format: string
  status: string
  episode_count: number | null
  cover_image: string
  banner_image: string
  cover_color: string
  season: string
  season_year: number | null
  is_adult: boolean
}

export interface DiscoveryRow {
  key: string
  title: string
  items: DiscoveryItem[]
}

export interface DiscoveryResponse {
  rows: DiscoveryRow[]
}

export interface TrackedResponse {
  in_progress: SeriesProgress[]
  completed: SeriesProgress[]
  paused: SeriesProgress[]
  dropped: SeriesProgress[]
}

export interface TrackResponse {
  series: SeriesProgress
  series_id: number
  feed_id: number
}

export interface AvailableEpisode {
  number: number
  title: string
  source_url: string
  size: number | null
  resolution: string
}

export interface AvailableResponse {
  episodes: AvailableEpisode[]
}

// ---- Full AniList detail (series detail page) ----

export interface AnilistDetailEpisode {
  number: number
  title: string | null
  thumbnail: string | null
  air_date: string | null
  overview: string | null
  runtime_min: number | null
}

export interface AnilistRelatedEntry {
  anilist_id: number
  relation_type?: string
  title_english: string | null
  title_romaji: string | null
  cover_image: string
  cover_color: string | null
  format: string | null
  status: string | null
}

export interface AnilistDetail {
  anilist_id: number
  title_english: string | null
  title_romaji: string | null
  cover_image: string | null
  cover_color: string | null
  banner_image: string | null
  format: string | null
  airing_status: string | null
  description: string
  genres: string[]
  average_score: number | null
  studio: string | null
  source_material: string | null
  season: string | null
  season_year: number | null
  duration_min: number | null
  episode_count: number | null
  next_airing: { episode: number; airing_at: number } | null
  trailer: { site: string; video_id: string; thumbnail: string } | null
  episodes: AnilistDetailEpisode[]
  relations: AnilistRelatedEntry[]
  recommendations: AnilistRelatedEntry[]
}

// ---- API methods ----

export const api = {
  // Series
  listSeries: () => get<SeriesProgress[]>('/series'),
  createSeries: (body: {
    anilist_id?: number
    title?: string
    season_number?: number
    default_profile_id?: number
  }) => post<SeriesDetail>('/series', body),
  getSeries: (id: number) => get<SeriesDetail>(`/series/${id}`),
  patchSeries: (id: number, body: {
    subscribed?: boolean
    favorite?: boolean
    season_number?: number
    default_profile_id?: number
    airing_status?: string
  }) => patch<SeriesDetail>(`/series/${id}`, body),
  deleteSeries: (id: number) => del<null>(`/series/${id}`),
  listEpisodes: (id: number) => get<EpisodeDetail[]>(`/series/${id}/episodes`),
  scanEpisodes: (id: number) => post<EpisodeDetail[]>(`/series/${id}/scan`),
  refreshSeries: (id: number) => post<unknown>(`/series/${id}/refresh`, {}),

  // Encode
  bulkEncode: (body: { episode_ids: number[]; profile_id?: number; resolutions?: number[] }) =>
    post<null>('/encode', body),
  encodeEpisode: (id: number) => post<null>(`/episodes/${id}/encode`),
  retryEpisode: (id: number) => post<null>(`/episodes/${id}/retry`),
  deleteEpisode: (id: number) => del<null>(`/episodes/${id}`),

  // AniList full detail (cover/genres/episodes/relations/recommendations)
  getAnilistDetail: (id: number) => get<AnilistDetail>(`/anilist/${id}/detail`),

  // Search
  searchAnilist: (q: string) => get<AnilistSearchResult[]>(`/search/anilist?q=${encodeURIComponent(q)}`),
  searchTorrents: (seriesId: number, episode?: number, provider?: string) => {
    const params = new URLSearchParams({ seriesId: String(seriesId) })
    if (episode != null) params.set('episode', String(episode))
    if (provider) params.set('provider', provider)
    return get<TorrentSearchResult[]>(`/search/torrents?${params}`)
  },

  // Feeds
  listFeeds: () => get<Feed[]>('/feeds'),
  createFeed: (body: Partial<Feed>) => post<Feed>('/feeds', body),
  patchFeed: (id: number, body: Partial<Feed>) => patch<Feed>(`/feeds/${id}`, body),
  deleteFeed: (id: number) => del<null>(`/feeds/${id}`),

  // Profiles
  listProfiles: () => get<Profile[]>('/profiles'),
  createProfile: (body: Partial<Profile> & { name: string }) => post<Profile>('/profiles', body),
  patchProfile: (id: number, body: Partial<Profile>) => patch<Profile>(`/profiles/${id}`, body),
  deleteProfile: (id: number) => del<null>(`/profiles/${id}`),
  getResolvedProfile: (id: number) => get<ResolvedProfile>(`/profiles/${id}/resolved`),

  // Settings
  getSettings: () => get<Settings>('/settings'),
  putSettings: (body: Settings) => put<Settings>('/settings', body),

  // Queue / Stats / Logs
  getQueue: () => get<QueueSnapshot>('/queue'),
  getStats: () => get<StatsResponse>('/stats'),
  getLogs: () => get<LogsResponse>('/logs'),

  // Discovery + tracking (discovery-first home)
  getDiscovery: () => get<DiscoveryResponse>('/discovery'),
  getTracked: () => get<TrackedResponse>('/tracked'),
  trackSeries: (body: { anilist_id: number }) => post<TrackResponse>('/track', body),
  pauseSeries: (id: number) => post<{ series: SeriesProgress }>(`/series/${id}/pause`, {}),
  dropSeries: (id: number) => post<{ series: SeriesProgress }>(`/series/${id}/drop`, {}),
  resumeSeries: (id: number) => post<{ series: SeriesProgress }>(`/series/${id}/resume`, {}),
  getAvailable: (id: number) => get<AvailableResponse>(`/series/${id}/available`),
  downloadAvailable: (id: number, body: { source_url: string; number: number; resolution?: string }) =>
    post<EpisodeDetail>(`/series/${id}/available/download`, body),

  // Extensions
  listExtensionRepos: () => get<unknown[]>('/extension-repos'),
  createExtensionRepo: (body: { name: string; url: string }) => post<unknown>('/extension-repos', body),
  installFromRepo: (id: number) => post<null>(`/extension-repos/${id}/install`),
  listExtensions: () => get<unknown[]>('/extensions'),
  enableExtension: (id: string) => post<null>(`/extensions/${id}/enable`),
  disableExtension: (id: string) => post<null>(`/extensions/${id}/disable`),
}

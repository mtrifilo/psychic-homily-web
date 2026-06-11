/**
 * Radio-related TypeScript types
 *
 * These types match the backend API response structures
 * for radio endpoints.
 */

// ============================================================================
// Radio Station
// ============================================================================

// PSY-669 / PSY-673: per-station network metadata. `is_flagship` is the
// bool on the station itself (radio_stations.is_flagship): true means
// THIS station is the network's primary/default broadcast. Frontend uses
// it to (a) hide non-flagship siblings from the /radio index, and (b)
// render the network tab bar (PSY-674) with the flagship as default.
export interface RadioNetworkInfo {
  slug: string
  name: string
  is_flagship: boolean
}

// PSY-669: other stations in the same network. Excludes self.
// Flagship-first sort; alphabetical within is_flagship buckets.
export interface RadioSiblingStation {
  id: number
  slug: string
  name: string
  broadcast_type: string
  frequency_mhz: number | null
  is_flagship: boolean
}

export interface RadioStationListItem {
  id: number
  name: string
  slug: string
  city: string | null
  state: string | null
  country: string | null
  broadcast_type: string
  frequency_mhz: number | null
  logo_url: string | null
  is_active: boolean
  network: RadioNetworkInfo | null
  sibling_stations: RadioSiblingStation[]
  show_count: number
}

// PSY-673: returns true when a station should appear on the /radio index.
// Network-less stations always render. Stations in a network render only
// when they're the flagship; non-flagship siblings reach their pages via
// the flagship's tab bar (PSY-674) or via their direct /radio/{slug} URL.
export function isStationVisibleOnIndex(station: {
  network: RadioNetworkInfo | null
}): boolean {
  if (!station.network) return true
  return station.network.is_flagship
}

// PSY-674: returns the canonical /radio detail URL for a station.
//   Network-less or flagship → /radio/{slug}
//   Sub-stream                → /radio/{network-slug}/channel/{local-slug}
// where local-slug = station.slug with the "{network-slug}-" prefix stripped.
// The "channel" literal segment disambiguates sub-streams from show pages,
// which live at /radio/{station-slug}/{show-slug}. Sub-stream slugs follow
// the network-prefix convention (wfmu-drummer, wfmu-rocknsoulradio,
// wfmu-sheena); a slug without the prefix falls back to the full slug
// rather than producing a malformed URL.
export function getStationDetailUrl(
  slug: string,
  network: { slug: string; is_flagship: boolean } | null
): string {
  if (!network || network.is_flagship) {
    return `/radio/${slug}`
  }
  const prefix = `${network.slug}-`
  const localSlug = slug.startsWith(prefix) ? slug.slice(prefix.length) : slug
  return `/radio/${network.slug}/channel/${localSlug}`
}

export interface RadioStationDetail {
  id: number
  name: string
  slug: string
  description: string | null
  city: string | null
  state: string | null
  country: string | null
  timezone: string | null
  stream_url: string | null
  stream_urls: Record<string, string> | null
  website: string | null
  donation_url: string | null
  donation_embed_url: string | null
  logo_url: string | null
  social: Record<string, string> | null
  broadcast_type: string
  frequency_mhz: number | null
  playlist_source: string | null
  playlist_config: Record<string, unknown> | null
  last_playlist_fetch_at: string | null
  is_active: boolean
  network: RadioNetworkInfo | null
  sibling_stations: RadioSiblingStation[]
  show_count: number
  created_at: string
  updated_at: string
}

export interface RadioStationsListResponse {
  stations: RadioStationListItem[]
  count: number
}

// ============================================================================
// Radio Show
// ============================================================================

export interface RadioShowListItem {
  id: number
  station_id: number
  station_name: string
  name: string
  slug: string
  host_name: string | null
  /** Human-readable air slot ("Mon 9pm-12am"); PSY-1050 shows directory. */
  schedule_display: string | null
  genre_tags: string[] | null
  image_url: string | null
  is_active: boolean
  episode_count: number
  /** Air date (YYYY-MM-DD) of the show's most recent episode (PSY-1048). */
  latest_air_date: string | null
}

export interface RadioShowDetail {
  id: number
  station_id: number
  station_name: string
  station_slug: string
  name: string
  slug: string
  host_name: string | null
  description: string | null
  schedule_display: string | null
  schedule: Record<string, unknown> | null
  genre_tags: string[] | null
  archive_url: string | null
  image_url: string | null
  is_active: boolean
  episode_count: number
  created_at: string
  updated_at: string
}

export interface RadioShowsListResponse {
  shows: RadioShowListItem[]
  count: number
}

// ============================================================================
// Radio Episode
// ============================================================================

export interface RadioEpisodeListItem {
  id: number
  show_id: number
  title: string | null
  air_date: string
  air_time: string | null
  duration_minutes: number | null
  archive_url: string | null
  play_count: number
  created_at: string
}

export interface RadioEpisodeDetail {
  id: number
  show_id: number
  show_name: string
  show_slug: string
  station_name: string
  station_slug: string
  title: string | null
  air_date: string
  air_time: string | null
  duration_minutes: number | null
  description: string | null
  archive_url: string | null
  mixcloud_url: string | null
  genre_tags: string[] | null
  mood_tags: string[] | null
  play_count: number
  plays: RadioPlay[]
  created_at: string
}

export interface RadioEpisodesListResponse {
  episodes: RadioEpisodeListItem[]
  total: number
}

// ============================================================================
// Radio Play
// ============================================================================

export interface RadioPlay {
  id: number
  episode_id: number
  position: number
  artist_name: string
  track_title: string | null
  album_title: string | null
  label_name: string | null
  release_year: number | null
  is_new: boolean
  rotation_status: string | null
  dj_comment: string | null
  is_live_performance: boolean
  is_request: boolean
  artist_id: number | null
  artist_slug: string | null
  release_id: number | null
  release_slug: string | null
  label_id: number | null
  label_slug: string | null
  musicbrainz_artist_id: string | null
  musicbrainz_recording_id: string | null
  musicbrainz_release_id: string | null
  air_timestamp: string | null
}

// ============================================================================
// Aggregation / Stats
// ============================================================================

export interface RadioTopArtist {
  artist_name: string
  artist_id: number | null
  artist_slug: string | null
  play_count: number
  episode_count: number
}

export interface RadioTopLabel {
  label_name: string
  label_id: number | null
  label_slug: string | null
  play_count: number
}

export interface RadioAsHeardOn {
  station_id: number
  station_name: string
  station_slug: string
  show_id: number
  show_name: string
  show_slug: string
  play_count: number
  last_played: string
}

export interface RadioNewReleaseRadarEntry {
  artist_name: string
  artist_id: number | null
  artist_slug: string | null
  album_title: string | null
  label_name: string | null
  release_id: number | null
  release_slug: string | null
  label_id: number | null
  label_slug: string | null
  play_count: number
  station_count: number
}

export interface RadioStats {
  total_stations: number
  total_shows: number
  total_episodes: number
  total_plays: number
  matched_plays: number
  unique_artists: number
}

// ============================================================================
// List response wrappers
// ============================================================================

export interface RadioTopArtistsResponse {
  artists: RadioTopArtist[]
  count: number
}

export interface RadioTopLabelsResponse {
  labels: RadioTopLabel[]
  count: number
}

export interface RadioAsHeardOnResponse {
  stations: RadioAsHeardOn[]
  count: number
}

export interface RadioNewReleasesResponse {
  releases: RadioNewReleaseRadarEntry[]
  count: number
}

// ============================================================================
// Constants
// ============================================================================

export const BROADCAST_TYPE_LABELS: Record<string, string> = {
  terrestrial: 'FM/AM',
  internet: 'Internet',
  both: 'FM/AM + Internet',
}

export const ROTATION_STATUS_LABELS: Record<string, string> = {
  heavy: 'Heavy Rotation',
  medium: 'Medium Rotation',
  light: 'Light Rotation',
  recommended_new: 'Recommended New',
  library: 'Library',
}

/**
 * Rotation-status tint, bound to the DS categorical palette (PSY-943). The
 * tokens map to a rough play-intensity gradient — heavy = chart-1 (orange,
 * hottest), medium = chart-3 (gold), light = chart-6 (denim, cooler),
 * recommended_new = chart-2 (green, "fresh"), library = muted (archival).
 * `--chart-4` (== --destructive) is intentionally avoided: a red rotation
 * pill would read as an error, not "heavy rotation".
 */
export const ROTATION_STATUS_COLORS: Record<string, string> = {
  heavy: 'bg-chart-1/15 text-chart-1 border-chart-1/20',
  medium: 'bg-chart-3/15 text-chart-3 border-chart-3/20',
  light: 'bg-chart-6/15 text-chart-6 border-chart-6/20',
  recommended_new: 'bg-chart-2/15 text-chart-2 border-chart-2/20',
  library: 'bg-muted text-muted-foreground border-border',
}

export function getBroadcastTypeLabel(type: string): string {
  return BROADCAST_TYPE_LABELS[type] ?? type
}

export function getRotationStatusLabel(status: string): string {
  return ROTATION_STATUS_LABELS[status] ?? status
}

export function getRotationStatusColor(status: string): string {
  return ROTATION_STATUS_COLORS[status] ?? 'bg-muted text-muted-foreground border-border'
}

// ============================================================================
// PSY-1048 aggregation shapes (PSY-1049 + PSY-1050 + PSY-1051)
//
// Mirrors backend/internal/services/contracts/radio.go.
// ============================================================================

/**
 * One artist in an episode row's short "played" preview: the raw playlist
 * name plus a knowledge-graph link when the matching engine resolved it.
 * Unmatched artists (null id/slug) render as plain text — never a dead link.
 */
export interface RadioEpisodePreviewArtist {
  artist_name: string
  artist_id: number | null
  artist_slug: string | null
}

/**
 * An episode row in the station-scoped and dial-wide latest-playlists feeds:
 * episode fields plus show and channel (station) attribution. A network
 * flagship's feed already includes its channels server-side.
 */
export interface RadioStationEpisodeRow {
  id: number
  title: string | null
  air_date: string
  play_count: number
  archive_url: string | null
  show_id: number
  show_name: string
  show_slug: string
  station_id: number
  station_name: string
  station_slug: string
  /** Always an array (may be empty); matched artists carry id + slug. */
  artist_preview: RadioEpisodePreviewArtist[]
}

/** Response shape for GET /radio/episodes/recent (and station episode feeds). */
export interface RadioRecentEpisodesResponse {
  episodes: RadioStationEpisodeRow[]
  total: number
}

/** Response shape for GET /radio-stations/{slug}/episodes. */
export interface RadioStationEpisodesResponse {
  episodes: RadioStationEpisodeRow[]
  total: number
}

// Declaration-merges into RadioEpisodeListItem above: episode list rows now
// carry the first few distinct playlist artists (PSY-1048).
export interface RadioEpisodeListItem {
  artist_preview: RadioEpisodePreviewArtist[]
}

// ============================================================================
// PSY-1022 now-playing shapes
//
// Mirrors RadioNowPlayingResponse in backend/internal/services/contracts/radio.go.
// ============================================================================

/** The matched our-DB show behind a now-playing payload (null = unmatched). */
export interface RadioNowPlayingShowRef {
  id: number
  name: string
  slug: string
  host_name: string | null
}

/**
 * The current track of a now-playing payload. Field names mirror RadioPlay
 * (minus id/episode_id/position) so track renderers work on both shapes.
 */
export interface RadioNowPlayingTrack {
  artist_name: string
  track_title: string | null
  album_title: string | null
  label_name: string | null
  release_year: number | null
  rotation_status: string | null
  dj_comment: string | null
  artist_id: number | null
  artist_slug: string | null
  release_id: number | null
  release_slug: string | null
  label_id: number | null
  label_slug: string | null
}

/**
 * GET /radio-stations/{slug}/now-playing. `source: 'live'` implies
 * `on_air: true` (the backend serves the archive fallback instead of a
 * half-live payload); `show` is null when the live show name couldn't be
 * matched to exactly one of the station's shows — render `show_name` as
 * plain text, never a dead link (PSY-1073).
 */
export interface RadioNowPlaying {
  source: 'live' | 'latest_archive'
  on_air: boolean
  show: RadioNowPlayingShowRef | null
  show_name: string | null
  /** Live-reported host (may be set even when `show` is unmatched). */
  host_name: string | null
  current_track: RadioNowPlayingTrack | null
  /** Up to 4 distinct previously-played artists, most recent first. */
  recent_artists: RadioEpisodePreviewArtist[]
  /** Air date (YYYY-MM-DD) of the fallback episode; null for live payloads. */
  episode_air_date: string | null
}

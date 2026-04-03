/**
 * Radio-related TypeScript types
 *
 * These types match the backend API response structures
 * for radio endpoints.
 */

// ============================================================================
// Radio Station
// ============================================================================

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
  show_count: number
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
  genre_tags: string[] | null
  image_url: string | null
  is_active: boolean
  episode_count: number
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

export const ROTATION_STATUS_COLORS: Record<string, string> = {
  heavy: 'bg-red-500/15 text-red-400 border-red-500/20',
  medium: 'bg-orange-500/15 text-orange-400 border-orange-500/20',
  light: 'bg-blue-500/15 text-blue-400 border-blue-500/20',
  recommended_new: 'bg-green-500/15 text-green-400 border-green-500/20',
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
